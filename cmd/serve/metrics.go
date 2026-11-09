package serve

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cerberauth/x/telemetryx"
	"github.com/getargvio/argvio-go"

	"github.com/cerberauth/scimply/config"
)

// Argvio product metrics for `scimply serve`, declared in the Argvio catalog.
// They live here, not in the server package, so that programs embedding
// scimply's server never send usage data.
//
// Every dimension value comes from a fixed list: never a path, a resource ID,
// a filter, a token or anything else from a request.
var (
	// What is deployed: how a server is configured, and how long it takes to
	// come up and how long it stays up.
	serveStarted  = argvio.NewCounter("serve.started", argvio.Dims("store", "auth", "tls"))
	serveStartup  = argvio.NewDuration("serve.startup", argvio.Dims("store"))
	serveUptime   = argvio.NewDuration("serve.uptime", argvio.Dims("store"))
	serveRequests = argvio.NewValue("serve.requests_per_run")

	// What it is used for: traffic by endpoint, method and outcome, and which
	// list/query options clients rely on.
	requestCounter  = argvio.NewCounter("scim.requests", argvio.Dims("endpoint", "method", "status"))
	requestDuration = argvio.NewDuration("scim.request", argvio.Dims("endpoint", "method"))
	errorCounter    = argvio.NewCounter("scim.errors", argvio.Dims("endpoint", "method", "code"))
	queryCounter    = argvio.NewCounter("scim.query", argvio.Dims("endpoint", "option"))

	storeTypes   = []string{storeMemory, storePostgres, storeMySQL, storeMongoDB, storeSCIM}
	authTypes    = []string{authNone, "bearer_token"}
	scimMethods  = []string{"get", "post", "put", "patch", "delete"}
	scimEndpoint = []string{"users", "groups", "me", "bulk", "serviceproviderconfig", "resourcetypes", "schemas"}
	// The error statuses SCIM defines (RFC 7644 §3.12), plus the ones a
	// server returns around them; anything else is "other".
	errorCodes = []string{"400", "401", "403", "404", "405", "409", "412", "413", "415", "429", "500", "501", "503"}
)

// queryOptions are the list/query parameters of RFC 7644 §3.4.2, by the name
// reported. Only whether one is present is reported, never its value.
var queryOptions = []struct {
	name   string
	params []string
}{
	{"filter", []string{"filter"}},
	{"sort", []string{"sortBy", "sortOrder"}},
	{"pagination", []string{"startIndex", "count"}},
	{"attributes", []string{"attributes"}},
	{"excluded_attributes", []string{"excludedAttributes"}},
}

// recordFeature is a seam for tests: telemetryx.Feature records on the client
// Instrument sets up, which a unit test does not have.
var recordFeature = telemetryx.Feature

// The defaults buildStore and buildAuth use for an empty type.
const (
	storeMemory   = "memory"
	storePostgres = "postgres"
	storeMySQL    = "mysql"
	storeMongoDB  = "mongodb"
	storeSCIM     = "scim"
	authNone      = "none"
)

func storeLabel(cfg *config.Config) string {
	if cfg.Store.Type == "" {
		return storeMemory
	}
	return telemetryx.Vocab(cfg.Store.Type, storeTypes...)
}

func authLabel(cfg *config.Config) string {
	if cfg.Auth.Type == "" {
		return authNone
	}
	return telemetryx.Vocab(cfg.Auth.Type, authTypes...)
}

// recordServeStarted counts a server start by how it is configured, times how
// long it took to come up, and notes the optional features it enables so they
// show up in Argvio's feature discovery. The defaults are not reported as
// features, only a deliberate departure from them.
func recordServeStarted(ctx context.Context, cfg *config.Config, startup time.Duration) {
	store := storeLabel(cfg)
	tls := "no"
	if cfg.Server.TLS.CertFile != "" && cfg.Server.TLS.KeyFile != "" {
		tls = "yes"
	}
	serveStarted.Add(ctx, 1, store, authLabel(cfg), tls)
	serveStartup.Record(ctx, startup, store)

	for feature, enabled := range map[string]bool{
		"serve.audit-log":        cfg.Audit.Type == "json",
		"serve.custom-base-path": cfg.Server.BasePath != config.DefaultBasePath,
		"serve.otel-export":      cfg.Telemetry.Endpoint != "",
		"serve.auto-migrate":     autoMigrates(cfg),
		"serve.scim-proxy-v1":    cfg.Store.Type == storeSCIM && (cfg.Store.SCIM.Version == "v1" || cfg.Store.SCIM.Version == "1.1"),
	} {
		if enabled {
			recordFeature(ctx, feature)
		}
	}
}

// autoMigrates reports whether the configured database store creates its own
// schema.
func autoMigrates(cfg *config.Config) bool {
	switch cfg.Store.Type {
	case storePostgres:
		return cfg.Store.Postgres.AutoMigrate
	case storeMySQL:
		return cfg.Store.MySQL.AutoMigrate
	case storeMongoDB:
		return cfg.Store.MongoDB.AutoMigrate
	}
	return false
}

// runStats counts what one `serve` run did, to report when it stops.
type runStats struct {
	requests atomic.Int64
}

// report records the run's totals. The request count is recorded even when it
// is zero: Argvio derives the share of servers that never served a request
// (the ones that were started and never connected to an IdP) from it.
func (s *runStats) report(ctx context.Context, cfg *config.Config, uptime time.Duration) {
	serveRequests.Record(ctx, float64(s.requests.Load()))
	serveUptime.Record(ctx, uptime, storeLabel(cfg))
}

// withRequestMetrics counts every SCIM request by endpoint, method and status
// class (2xx, 4xx, 5xx), times it by endpoint and method, counts failures by
// exact status and notes which query options it used. stats may be nil.
func withRequestMetrics(ctx context.Context, basePath string, stats *runStats, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		elapsed := time.Since(start)

		endpoint := scimEndpointOf(r.URL.Path, basePath)
		method := telemetryx.Vocab(r.Method, scimMethods...)
		requestCounter.Add(ctx, 1, endpoint, method, statusClass(rec.status))
		requestDuration.Record(ctx, elapsed, endpoint, method)
		if rec.status >= http.StatusBadRequest {
			errorCounter.Add(ctx, 1, endpoint, method, telemetryx.Vocab(strconv.Itoa(rec.status), errorCodes...))
		}
		for _, option := range queryOptionsOf(r) {
			queryCounter.Add(ctx, 1, endpoint, option)
		}
		if stats != nil {
			stats.requests.Add(1)
		}
	})
}

// queryOptionsOf names the list/query options r uses. A search done with
// POST .search carries its options in the body, which is not read: it is
// reported as the "search" option alone.
func queryOptionsOf(r *http.Request) []string {
	var used []string
	if strings.HasSuffix(r.URL.Path, "/.search") {
		used = append(used, "search")
	}
	if r.URL.RawQuery == "" {
		return used
	}
	query := r.URL.Query()
	for _, option := range queryOptions {
		for _, param := range option.params {
			if query.Has(param) {
				used = append(used, option.name)
				break
			}
		}
	}
	return used
}

// scimEndpointOf names the SCIM endpoint path addresses: its first segment
// after basePath, lower-cased, when it is a standard one, else "other".
func scimEndpointOf(path, basePath string) string {
	rest, ok := strings.CutPrefix(path, basePath)
	if !ok {
		return "other"
	}
	first, _, _ := strings.Cut(strings.TrimPrefix(rest, "/"), "/")
	return telemetryx.Vocab(first, scimEndpoint...)
}

func statusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}

// statusRecorder remembers the status code a handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status, s.wrote = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wrote = true
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
