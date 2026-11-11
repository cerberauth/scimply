package server_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cerberauth/scimply/resource"
	"github.com/cerberauth/scimply/schema"
	"github.com/cerberauth/scimply/server"
	"github.com/cerberauth/scimply/store"
)

type panicStore struct {
	store.ResourceStore
}

func (panicStore) Get(context.Context, string, string) (*resource.Resource, error) {
	panic("secret internal detail")
}

func newLoggedServer(t *testing.T, st store.ResourceStore, opts ...server.Option) (*server.Server, *bytes.Buffer) {
	t.Helper()
	reg := schema.NewRegistry()
	reg.RegisterDefaults()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	s, err := server.New(append([]server.Option{
		server.WithStore(st),
		server.WithSchemaRegistry(reg),
		server.WithLogger(logger),
	}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return s, &buf
}

func TestRecovery_DoesNotLeakPanicAndLogsIt(t *testing.T) {
	s, logs := newLoggedServer(t, panicStore{store.NewMemoryStore()})

	resp := doRequest(t, s, http.MethodGet, "/Users/123", "")
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
	if strings.Contains(body, "secret internal detail") {
		t.Errorf("panic value leaked to client: %s", body)
	}
	out := logs.String()
	if !strings.Contains(out, "secret internal detail") || !strings.Contains(out, "stack=") {
		t.Errorf("panic and stack not logged: %s", out)
	}
}

func TestMiddlewareOrder_RejectionsAreLogged(t *testing.T) {
	s, logs := newLoggedServer(t, store.NewMemoryStore(), server.WithBearerTokens("good"))

	resp := doRequest(t, s, http.MethodGet, "/Users", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}

	req := httptest.NewRequest(http.MethodPost, "/Users", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer good")
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rr.Code)
	}

	out := logs.String()
	for _, status := range []string{"status=401", "status=415"} {
		if !strings.Contains(out, status) {
			t.Errorf("expected request log with %s, got: %s", status, out)
		}
	}
}

func TestWithBaseURL_IgnoresHostHeader(t *testing.T) {
	s := newTestServer(t, server.WithBaseURL("https://idp.example.com/scim/v2/"))

	req := httptest.NewRequest(http.MethodPost, "/Users", strings.NewReader(`{"userName":"alice"}`))
	req.Host = "evil.example.net"
	req.Header.Set("Content-Type", "application/scim+json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://idp.example.com/scim/v2/Users/") {
		t.Errorf("Location: got %q", loc)
	}
	if !strings.Contains(rr.Body.String(), `"location":"`+loc+`"`) {
		t.Errorf("meta.location does not match Location header: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/ResourceTypes/User", nil)
	req.Host = "evil.example.net"
	rr = httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if loc := rr.Header().Get("Location"); loc != "https://idp.example.com/scim/v2/ResourceTypes/User" {
		t.Errorf("ResourceType Location: got %q", loc)
	}
}

func TestWithBearerTokens(t *testing.T) {
	s := newTestServer(t, server.WithBearerTokens("token-a", "", "token-b"))

	for token, want := range map[string]int{
		"token-a": http.StatusOK,
		"token-b": http.StatusOK,
		"token-c": http.StatusUnauthorized,
		"":        http.StatusUnauthorized,
		"token":   http.StatusUnauthorized,
	} {
		req := httptest.NewRequest(http.MethodGet, "/Users", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != want {
			t.Errorf("token %q: expected %d, got %d", token, want, rr.Code)
		}
	}
}
