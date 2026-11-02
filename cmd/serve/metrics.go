package serve

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cerberauth/x/telemetryx"

	"github.com/cerberauth/scimply/config"
)

// Argvio product metrics for `scimply serve`, declared in argvio.metrics.yaml.
// They live here, not in the server package, so that programs embedding
// scimply's server never send usage data.
//
// Every dimension value comes from a fixed list: never a path, a resource ID,
// a filter, a token or anything else from a request.
var (
	serveStarted    = telemetryx.NewCounter("serve.started", "store", "auth", "tls")
	requestCounter  = telemetryx.NewCounter("scim.requests", "endpoint", "method", "status")
	requestDuration = telemetryx.NewDuration("scim.request", "endpoint", "method")

	storeTypes   = []string{storeMemory, "postgres", "mysql", "mongodb", "scim"}
	authTypes    = []string{authNone, "bearer_token"}
	scimMethods  = []string{"get", "post", "put", "patch", "delete"}
	scimEndpoint = []string{"users", "groups", "me", "bulk", "serviceproviderconfig", "resourcetypes", "schemas"}
)

// The defaults buildStore and buildAuth use for an empty type.
const (
	storeMemory = "memory"
	authNone    = "none"
)

// recordServeStarted counts a server start by how it is configured.
func recordServeStarted(ctx context.Context, cfg *config.Config) {
	store := cfg.Store.Type
	if store == "" {
		store = storeMemory
	}
	auth := cfg.Auth.Type
	if auth == "" {
		auth = authNone
	}
	tls := "no"
	if cfg.Server.TLS.CertFile != "" && cfg.Server.TLS.KeyFile != "" {
		tls = "yes"
	}
	serveStarted.Add(ctx, 1, telemetryx.Vocab(store, storeTypes...), telemetryx.Vocab(auth, authTypes...), tls)
}

// withRequestMetrics counts every SCIM request by endpoint, method and status
// class (2xx, 4xx, 5xx), and times it by endpoint and method.
func withRequestMetrics(ctx context.Context, basePath string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		endpoint := scimEndpointOf(r.URL.Path, basePath)
		method := telemetryx.Vocab(r.Method, scimMethods...)
		requestCounter.Add(ctx, 1, endpoint, method, statusClass(rec.status))
		requestDuration.Record(ctx, time.Since(start), endpoint, method)
	})
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
