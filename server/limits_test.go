package server_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cerberauth/scimply/server"
)

func TestMaxBodySize(t *testing.T) {
	s := newTestServer(t, server.WithMaxBodySize(64))
	big := `{"userName":"alice","displayName":"` + strings.Repeat("x", 128) + `"}`

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/Users"},
		{http.MethodPut, "/Users/some-id"},
		{http.MethodPatch, "/Users/some-id"},
		{http.MethodPost, "/Users/.search"},
	} {
		resp := doRequest(t, s, tc.method, tc.path, big)
		if resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Errorf("%s %s: expected 413, got %d", tc.method, tc.path, resp.StatusCode)
		}
		resp.Body.Close()
	}

	resp := doRequest(t, s, http.MethodPost, "/Users", `{"userName":"alice"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("small body: expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestMaxBulkPayloadSize(t *testing.T) {
	s := newTestServer(t, server.WithMaxBulkPayloadSize(128))

	body := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:BulkRequest"],"Operations":[` +
		`{"method":"POST","path":"/Users","bulkId":"b1","data":{"userName":"` + strings.Repeat("x", 200) + `"}}]}`
	resp := doRequest(t, s, http.MethodPost, "/Bulk", body)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doRequest(t, s, http.MethodGet, "/ServiceProviderConfig", "")
	var spc struct {
		Bulk struct {
			MaxPayloadSize int `json:"maxPayloadSize"`
		} `json:"bulk"`
	}
	decodeJSON(t, resp, &spc)
	if spc.Bulk.MaxPayloadSize != 128 {
		t.Errorf("bulk.maxPayloadSize: expected 128, got %d", spc.Bulk.MaxPayloadSize)
	}
}

func TestBulkRejectsNonResourcePaths(t *testing.T) {
	s := newTestServer(t)

	for _, path := range []string{
		"/Bulk",
		"/ServiceProviderConfig",
		"/Schemas",
		"/ResourceTypes",
		"/Users/.search",
		"/Users/id/extra",
		"/Unknown",
		"Users",
	} {
		body := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:BulkRequest"],"Operations":[` +
			`{"method":"POST","path":"` + path + `","data":{"schemas":["urn:ietf:params:scim:api:messages:2.0:BulkRequest"],"Operations":[]}}]}`
		resp := doRequest(t, s, http.MethodPost, "/Bulk", body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", path, resp.StatusCode)
		}
		var bulk struct {
			Operations []struct {
				Status   string                 `json:"status"`
				Response map[string]interface{} `json:"response"`
			} `json:"Operations"`
		}
		decodeJSON(t, resp, &bulk)
		if len(bulk.Operations) != 1 {
			t.Fatalf("%s: expected 1 operation result, got %d", path, len(bulk.Operations))
		}
		op := bulk.Operations[0]
		if op.Status != "400" || op.Response["scimType"] != "invalidPath" {
			t.Errorf("%s: expected 400 invalidPath, got %s %v", path, op.Status, op.Response)
		}
	}
}
