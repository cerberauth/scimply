package serve

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	"github.com/spf13/cobra"
)

var configFile string

func NewServeCmd() (serveCmd *cobra.Command) {
	serveCmd = &cobra.Command{
		Use:   "serve",
		Short: "Start the SCIM server",
		RunE:  runServe,
	}

	serveCmd.Flags().StringVarP(&configFile, "config", "c", "scimply.yaml", "path to config file")

	return serveCmd
}

type storeCloser interface {
	Close(ctx context.Context) error
}

func runServe(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(configFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := buildLogger(cfg.Log)

	auditLogger, err := buildAuditLogger(cfg.Audit)
	if err != nil {
		return err
	}

	st, closeStore, err := buildStore(cfg)
	if err != nil {
		return err
	}

	authOpts, err := buildAuth(cfg.Auth)
	if err != nil {
		return err
	}

	reg := schema.NewRegistry()
	reg.RegisterDefaults()

	opts := []server.Option{
		server.WithStore(st),
		server.WithSchemaRegistry(reg),
		server.WithBasePath(cfg.Server.BasePath),
		server.WithLogger(logger),
		server.WithAuditLogger(auditLogger),
		server.WithDefaultPageSize(cfg.Server.DefaultPageSize),
		server.WithMaxPageSize(cfg.Server.MaxPageSize),
		server.WithMaxBulkOps(cfg.Server.MaxBulkOps),
	}
	opts = append(opts, authOpts...)

	srv, err := server.New(opts...)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	httpSrv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           srv,
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
			errCh <- httpSrv.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
		} else {
			errCh <- httpSrv.ListenAndServe()
		}
	}()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
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
	var handler slog.Handler
	if cfg.Format == "text" {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}
	return slog.New(handler)
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

	case "postgres":
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

	case "mysql":
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

	case "mongodb":
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

	case "scim":
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
		tokens := make(map[string]struct{}, len(cfg.Tokens))
		for _, t := range cfg.Tokens {
			tokens[t] = struct{}{}
		}
		return []server.Option{
			server.WithBearerTokenAuth(func(token string) (bool, error) {
				_, ok := tokens[token]
				return ok, nil
			}),
		}, nil

	default:
		return nil, fmt.Errorf("unknown auth type %q", cfg.Type)
	}
}
