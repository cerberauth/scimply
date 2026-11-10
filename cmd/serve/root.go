package serve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cerberauth/x/otelx"
	"github.com/cerberauth/x/telemetryx"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/bridges/otelslog"

	"github.com/cerberauth/scimply/audit"
	"github.com/cerberauth/scimply/config"
	"github.com/cerberauth/scimply/connector/mongodb"
	"github.com/cerberauth/scimply/connector/mysql"
	"github.com/cerberauth/scimply/connector/postgres"
	scimconnector "github.com/cerberauth/scimply/connector/scim"
	"github.com/cerberauth/scimply/protocol"
	"github.com/cerberauth/scimply/schema"
	"github.com/cerberauth/scimply/server"
	"github.com/cerberauth/scimply/store"
)

func NewServeCmd(version, commit, date string) *cobra.Command {
	var configFile string
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the SCIM server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd, configFile, version, commit, date)
		},
	}
	serveCmd.Flags().StringVarP(&configFile, "config", "c", "scimply.yaml", "path to config file")
	return serveCmd
}

type storeCloser interface {
	Close(ctx context.Context) error
}

// runServe's failures carry a fixed reason (see telemetryx.Reason), so Argvio
// can tell where users get stuck setting up a server. The reason never
// includes the error text.
func runServe(cmd *cobra.Command, configFile, version, commit, date string) error {
	began := time.Now()

	cfg, err := config.Load(configFile)
	if err != nil {
		return telemetryx.Reason(fmt.Errorf("load config: %w", err), "config")
	}

	xOpts := []otelx.Option{
		otelx.WithCommit(commit),
		otelx.WithBuildDate(date),
	}
	if cfg.Telemetry.Endpoint != "" {
		xOpts = append(xOpts, otelx.WithEndpoint(cfg.Telemetry.Endpoint))
	}
	if cfg.Telemetry.Headers != nil {
		xOpts = append(xOpts, otelx.WithHeaders(cfg.Telemetry.Headers))
	}
	otelShutdown, otelErr := otelx.New(cmd.Context(), "scimply", version, xOpts...)
	if otelErr != nil {
		otelShutdown = nil
	}
	if otelShutdown != nil {
		defer func() {
			shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = otelShutdown(shutCtx)
		}()
	}

	logger := buildLogger(cfg.Log)

	auditLogger, err := buildAuditLogger(cfg.Audit)
	if err != nil {
		return telemetryx.Reason(err, "audit-log")
	}

	st, closeStore, err := buildStore(cfg)
	if err != nil {
		return telemetryx.Reason(err, "store")
	}

	authOpts, err := buildAuth(cfg.Auth)
	if err != nil {
		return telemetryx.Reason(err, "auth")
	}

	reg := schema.NewRegistry()
	reg.RegisterDefaults()

	opts := []server.Option{
		server.WithStore(store.NewTracingStore(st)),
		server.WithSchemaRegistry(reg),
		server.WithBasePath(cfg.Server.BasePath),
		server.WithBaseURL(cfg.Server.BaseURL),
		server.WithLogger(logger),
		server.WithAuditLogger(auditLogger),
		server.WithDefaultPageSize(cfg.Server.DefaultPageSize),
		server.WithMaxPageSize(cfg.Server.MaxPageSize),
		server.WithMaxBulkOps(cfg.Server.MaxBulkOps),
	}
	opts = append(opts, authOpts...)

	srv, err := server.New(opts...)
	if err != nil {
		return telemetryx.Reason(fmt.Errorf("create server: %w", err), "server")
	}

	// Bind the port before reporting the start, so a server that could not
	// listen (port in use) is not counted as started.
	addr := cfg.Server.Addr
	if addr == "" {
		addr = ":http" // what http.Server.ListenAndServe falls back to
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return telemetryx.Reason(fmt.Errorf("server error: %w", err), "listen")
	}
	recordServeStarted(cmd.Context(), cfg, time.Since(began))
	listening := time.Now()
	stats := &runStats{}
	defer func() { stats.report(cmd.Context(), cfg, time.Since(listening)) }()

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           withRequestMetrics(cmd.Context(), cfg.Server.BasePath, stats, srv),
		ReadTimeout:       cfg.Server.ReadTimeout.Duration,
		WriteTimeout:      cfg.Server.WriteTimeout.Duration,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", cfg.Server.Addr, "base_path", cfg.Server.BasePath)
		if cfg.Server.TLS.CertFile != "" && cfg.Server.TLS.KeyFile != "" {
			errCh <- httpSrv.ServeTLS(ln, cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
		} else {
			errCh <- httpSrv.Serve(ln)
		}
	}()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return telemetryx.Reason(fmt.Errorf("server error: %w", err), "serve")
		}
	case <-ctx.Done():
		logger.Info("shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout.Duration)
		defer cancel()
		if err := httpSrv.Shutdown(shutCtx); err != nil {
			logger.Error("shutdown error", "err", err)
		}
		if closeStore != nil {
			if err := closeStore.Close(shutCtx); err != nil {
				logger.Error("store close error", "err", err)
			}
		}
	}

	return nil
}

type multiHandler []slog.Handler

func (m multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range m {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make(multiHandler, len(m))
	for i, h := range m {
		handlers[i] = h.WithAttrs(attrs)
	}
	return handlers
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	handlers := make(multiHandler, len(m))
	for i, h := range m {
		handlers[i] = h.WithGroup(name)
	}
	return handlers
}

func buildLogger(cfg config.LogConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	var stderrHandler slog.Handler
	if cfg.Format == "text" {
		stderrHandler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		stderrHandler = slog.NewJSONHandler(os.Stderr, opts)
	}

	return slog.New(multiHandler{
		stderrHandler,
		otelslog.NewHandler("scimply"),
	})
}

func buildAuditLogger(cfg config.AuditConfig) (audit.Logger, error) {
	if cfg.Type != "json" {
		return audit.Noop(), nil
	}

	var w io.Writer
	switch cfg.Output {
	case "stderr":
		w = os.Stderr
	case "", "stdout":
		w = os.Stdout
	default:
		f, err := os.OpenFile(cfg.Output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return nil, fmt.Errorf("open audit log %q: %w", cfg.Output, err)
		}
		w = f
	}

	return audit.NewJSONLogger(w), nil
}

func buildStore(cfg *config.Config) (store.ResourceStore, storeCloser, error) {
	switch cfg.Store.Type {
	case "", "memory":
		return store.NewMemoryStore(), nil, nil

	case storePostgres:
		pc := cfg.Store.Postgres
		s, err := postgres.New(
			postgres.WithDSN(pc.DSN),
			postgres.WithTablePrefix(pc.TablePrefix),
			postgres.WithMaxConns(pc.MaxConns),
			postgres.WithMinConns(pc.MinConns),
			postgres.WithConnTimeout(pc.ConnTimeout.Duration),
			postgres.WithAutoMigrate(pc.AutoMigrate),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("create postgres store: %w", err)
		}
		if err := s.Init(context.Background()); err != nil {
			return nil, nil, fmt.Errorf("init postgres store: %w", err)
		}
		return s, s, nil

	case storeMySQL:
		mc := cfg.Store.MySQL
		s, err := mysql.New(
			mysql.WithDSN(mc.DSN),
			mysql.WithTablePrefix(mc.TablePrefix),
			mysql.WithMaxConns(mc.MaxConns),
			mysql.WithAutoMigrate(mc.AutoMigrate),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("create mysql store: %w", err)
		}
		if err := s.Init(context.Background()); err != nil {
			return nil, nil, fmt.Errorf("init mysql store: %w", err)
		}
		return s, s, nil

	case storeMongoDB:
		mdc := cfg.Store.MongoDB
		s, err := mongodb.New(
			mongodb.WithURI(mdc.URI),
			mongodb.WithDatabase(mdc.Database),
			mongodb.WithCollectionPrefix(mdc.CollectionPrefix),
			mongodb.WithTimeout(mdc.Timeout.Duration),
			mongodb.WithAutoMigrate(mdc.AutoMigrate),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("create mongodb store: %w", err)
		}
		if err := s.Init(context.Background()); err != nil {
			return nil, nil, fmt.Errorf("init mongodb store: %w", err)
		}
		return s, s, nil

	case storeSCIM:
		sc := cfg.Store.SCIM
		scimOpts := []scimconnector.Option{
			scimconnector.WithBaseURL(sc.BaseURL),
			scimconnector.WithTimeout(sc.Timeout.Duration),
		}
		if sc.Token != "" {
			scimOpts = append(scimOpts, scimconnector.WithBearerToken(sc.Token))
		}
		if sc.Version == "v1" || sc.Version == "1.1" {
			scimOpts = append(scimOpts, scimconnector.WithVersion(protocol.V1_1))
		}
		s, err := scimconnector.New(scimOpts...)
		if err != nil {
			return nil, nil, fmt.Errorf("create scim connector: %w", err)
		}
		return s, nil, nil

	default:
		return nil, nil, fmt.Errorf("unknown store type %q", cfg.Store.Type)
	}
}

func buildAuth(cfg config.AuthConfig) ([]server.Option, error) {
	switch cfg.Type {
	case "", "none":
		return nil, nil

	case "bearer_token":
		tokens, err := cfg.ResolveTokens()
		if err != nil {
			return nil, err
		}
		if len(tokens) == 0 {
			return nil, fmt.Errorf("bearer_token auth requires at least one token")
		}
		return []server.Option{server.WithBearerTokens(tokens...)}, nil

	default:
		return nil, fmt.Errorf("unknown auth type %q", cfg.Type)
	}
}
