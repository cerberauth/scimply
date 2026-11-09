package serve

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getargvio/argvio-go"

	"github.com/cerberauth/scimply/config"
)

func TestScimEndpointOf(t *testing.T) {
	tests := []struct{ path, base, want string }{
		{"/Users", "", "users"},
		{"/Users/2819c223-7f76-453a-919d-413861904646", "", "users"},
		{"/scim/v2/Groups/.search", "/scim/v2", "groups"},
		{"/scim/v2/ServiceProviderConfig", "/scim/v2", "serviceproviderconfig"},
		{"/Bulk", "", "bulk"},
		{"/CustomThings/42", "", "other"},
		{"/elsewhere/Users", "/scim/v2", "other"},
		{"/", "", "other"},
	}
	for _, tt := range tests {
		if got := scimEndpointOf(tt.path, tt.base); got != tt.want {
			t.Errorf("scimEndpointOf(%q, %q) = %q, want %q", tt.path, tt.base, got, tt.want)
		}
	}
}

func TestStatusClass(t *testing.T) {
	for code, want := range map[int]string{200: "2xx", 201: "2xx", 204: "2xx", 304: "3xx", 404: "4xx", 409: "4xx", 500: "5xx"} {
		if got := statusClass(code); got != want {
			t.Errorf("statusClass(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestQueryOptionsOf(t *testing.T) {
	tests := []struct {
		name, method, target string
		want                 []string
	}{
		{"no query", http.MethodGet, "/scim/v2/Users", nil},
		{"filter", http.MethodGet, "/scim/v2/Users?filter=userName+eq+%22bjensen%22", []string{"filter"}},
		{"pagination counts once", http.MethodGet, "/scim/v2/Users?startIndex=1&count=10", []string{"pagination"}},
		{"sort by either parameter", http.MethodGet, "/scim/v2/Users?sortOrder=descending", []string{"sort"}},
		{"projection", http.MethodGet, "/scim/v2/Users?attributes=userName&excludedAttributes=groups", []string{"attributes", "excluded_attributes"}},
		{"search", http.MethodPost, "/scim/v2/Users/.search", []string{"search"}},
		{"unknown parameters are ignored", http.MethodGet, "/scim/v2/Users?token=secret&x=1", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryOptionsOf(httptest.NewRequest(tt.method, tt.target, nil))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("queryOptionsOf() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStoreAndAuthLabels(t *testing.T) {
	cfg := &config.Config{}
	if got := storeLabel(cfg); got != "memory" {
		t.Errorf("empty store = %q, want memory", got)
	}
	if got := authLabel(cfg); got != "none" {
		t.Errorf("empty auth = %q, want none", got)
	}
	cfg.Store.Type, cfg.Auth.Type = "Postgres", "something-custom"
	if got := storeLabel(cfg); got != "postgres" {
		t.Errorf("store = %q, want postgres", got)
	}
	if got := authLabel(cfg); got != "other" {
		t.Errorf("unlisted auth = %q, want other", got)
	}
}

// The middleware passes the response through unchanged and is a no-op for
// telemetry until Instrument has run.
func TestWithRequestMetrics_PassesThrough(t *testing.T) {
	stats := &runStats{}
	h := withRequestMetrics(context.Background(), "", stats, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/Users", nil))
	if rec.Code != http.StatusCreated || rec.Body.String() != "ok" {
		t.Errorf("got %d %q, want 201 \"ok\"", rec.Code, rec.Body.String())
	}
	if got := stats.requests.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}

	sr := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	_, _ = sr.Write([]byte("x"))
	sr.WriteHeader(http.StatusTeapot) // too late: the implicit 200 was sent
	if sr.status != http.StatusOK {
		t.Errorf("expected the first status to win, got %d", sr.status)
	}
}

// collector stands in for the Argvio ingest endpoint and keeps what is sent.
type collector struct {
	mu     sync.Mutex
	bodies map[string]string
}

func (c *collector) get(path string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bodies[path]
}

// useCollector points the default Argvio client, which the package-level
// metrics record on, at a local collector, as an opted-in interactive user.
func useCollector(t *testing.T) (*collector, *argvio.Client) {
	t.Helper()
	// A developer's own opt-outs, or a CI environment, must not change what
	// the client under test sends.
	for _, name := range []string{"DO_NOT_TRACK", "ARGVIO_DISABLED"} {
		t.Setenv(name, "")
	}
	c := &collector{bodies: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.bodies[r.URL.Path] += string(b)
		c.mu.Unlock()
	}))
	t.Cleanup(srv.Close)

	client, err := argvio.New("argv_live_pub_test", "scimply", "test",
		argvio.WithEndpoint(srv.URL),
		argvio.WithHTTPTransport(),
		argvio.WithDisableOTELEnvFallback(),
		argvio.WithConsentProvider(argvio.StaticProvider{Tier: argvio.TierBasic}),
		argvio.WithInstallIDPath(filepath.Join(t.TempDir(), "install-id")),
	)
	if err != nil {
		t.Fatalf("argvio.New: %v", err)
	}
	t.Cleanup(func() { argvio.SetDefault(nil) })
	return c, client
}

// What reaches Argvio: the metrics `serve` declares, with the dimension values
// they are meant to carry, and nothing a client or an operator chose.
func TestServeMetrics_AreSentWithoutRequestContent(t *testing.T) {
	coll, client := useCollector(t)
	ctx := context.Background()

	cfg := &config.Config{}
	cfg.Store.Type = "postgres"
	cfg.Store.Postgres.AutoMigrate = true
	cfg.Auth.Type = "bearer_token"
	cfg.Audit.Type = "json"
	cfg.Server.BasePath = "/scim/v2"
	cfg.Telemetry.Endpoint = "https://otel.internal.example.com"
	cfg.Store.Postgres.DSN = "postgres://admin:hunter2@db.internal.example.com/scim"
	recordServeStarted(ctx, cfg, 250*time.Millisecond)

	stats := &runStats{}
	h := withRequestMetrics(ctx, cfg.Server.BasePath, stats, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "secret-id-123") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusConflict)
	}))
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/scim/v2/Users", nil),
		httptest.NewRequest(http.MethodGet, "/scim/v2/Users/secret-id-123?filter=userName+eq+%22secret-user%22", nil),
		httptest.NewRequest(http.MethodGet, "/scim/v2/Widgets?count=5", nil),
	} {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	stats.report(ctx, cfg, 3*time.Second)

	if err := client.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	metrics, logs := coll.get("/v1/metrics"), coll.get("/v1/logs")

	for _, want := range []string{
		"serve.started", "serve.startup", "serve.uptime", "serve.requests_per_run",
		"scim.requests", "scim.request", "scim.errors", "scim.query",
		"postgres", "bearer_token", "users", "other", "post", "get", "4xx", "404", "409", "filter", "pagination",
	} {
		if !strings.Contains(metrics, want) {
			t.Errorf("expected %q in the metrics sent", want)
		}
	}
	for _, leaked := range []string{"secret-id-123", "secret-user", "hunter2", "internal.example.com", "Widgets"} {
		if strings.Contains(metrics, leaked) || strings.Contains(logs, leaked) {
			t.Errorf("%q must never be sent", leaked)
		}
	}
}

// Optional features are reported only when deliberately turned on, by fixed
// names, so Argvio's feature discovery shows what is actually used.
func TestRecordServeStarted_ReportsOnlyEnabledFeatures(t *testing.T) {
	var got []string
	orig := recordFeature
	recordFeature = func(_ context.Context, name string) { got = append(got, name) }
	t.Cleanup(func() { recordFeature = orig })

	cfg := &config.Config{}
	cfg.Server.BasePath = config.DefaultBasePath
	recordServeStarted(context.Background(), cfg, time.Second)
	if len(got) != 0 {
		t.Errorf("defaults reported features %v, want none", got)
	}

	cfg.Server.BasePath = "/identity/scim"
	cfg.Audit.Type = "json"
	cfg.Telemetry.Endpoint = "https://otel.internal.example.com"
	cfg.Store.Type = "scim"
	cfg.Store.SCIM.Version = "v1"
	recordServeStarted(context.Background(), cfg, time.Second)
	slices.Sort(got)
	want := []string{"serve.audit-log", "serve.custom-base-path", "serve.otel-export", "serve.scim-proxy-v1"}
	if !slices.Equal(got, want) {
		t.Errorf("features = %v, want %v", got, want)
	}
}

// A server that serves nothing still reports its request count (zero), which
// is how Argvio knows how many servers were started and never used.
func TestRunStats_ReportsZeroRequests(t *testing.T) {
	coll, client := useCollector(t)
	(&runStats{}).report(context.Background(), &config.Config{}, time.Second)
	if err := client.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if !strings.Contains(coll.get("/v1/metrics"), "serve.requests_per_run") {
		t.Error("expected serve.requests_per_run to be sent even when zero")
	}
}
