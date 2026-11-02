package serve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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

// The middleware passes the response through unchanged and is a no-op for
// telemetry until Instrument has run.
func TestWithRequestMetrics_PassesThrough(t *testing.T) {
	h := withRequestMetrics(context.Background(), "", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/Users", nil))
	if rec.Code != http.StatusCreated || rec.Body.String() != "ok" {
		t.Errorf("got %d %q, want 201 \"ok\"", rec.Code, rec.Body.String())
	}

	sr := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	_, _ = sr.Write([]byte("x"))
	sr.WriteHeader(http.StatusTeapot) // too late: the implicit 200 was sent
	if sr.status != http.StatusOK {
		t.Errorf("expected the first status to win, got %d", sr.status)
	}
}
