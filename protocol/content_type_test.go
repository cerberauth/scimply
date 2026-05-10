package protocol

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newRequest(accept string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/Users", nil)
	if accept != "" {
		r.Header.Set("Accept", accept)
	}
	return r
}

func TestNegotiateContentType(t *testing.T) {
	tests := []struct {
		accept string
		want   string
	}{
		{"", ContentTypeSCIM},
		{"application/scim+json", ContentTypeSCIM},
		{"application/json", ContentTypeJSON},
		{"application/json, application/scim+json", ContentTypeJSON},
		{"application/scim+json, application/json", ContentTypeSCIM},
		{"*/*", ContentTypeSCIM},
		{"application/*", ContentTypeSCIM},
		{"text/html", ContentTypeSCIM},
		{"application/json;q=0.9, application/scim+json;q=1.0", ContentTypeJSON},
	}
	for _, tc := range tests {
		r := newRequest(tc.accept)
		got := NegotiateContentType(r)
		if got != tc.want {
			t.Errorf("NegotiateContentType(Accept=%q) = %q, want %q", tc.accept, got, tc.want)
		}
	}
}

func TestSetContentType(t *testing.T) {
	w := httptest.NewRecorder()
	SetContentType(w)
	if got := w.Header().Get("Content-Type"); got != ContentTypeSCIM {
		t.Errorf("SetContentType set %q, want %q", got, ContentTypeSCIM)
	}
}

func TestIsAcceptable(t *testing.T) {
	tests := []struct {
		accept string
		want   bool
	}{
		{"", true},
		{"application/scim+json", true},
		{"application/json", true},
		{"*/*", true},
		{"application/*", true},
		{"text/html", false},
		{"text/plain, application/json", true},
		{"text/plain", false},
	}
	for _, tc := range tests {
		r := newRequest(tc.accept)
		got := IsAcceptable(r)
		if got != tc.want {
			t.Errorf("IsAcceptable(Accept=%q) = %v, want %v", tc.accept, got, tc.want)
		}
	}
}
