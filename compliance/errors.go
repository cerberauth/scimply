package compliance

import (
	"net/http"
	"testing"
)

func testNotFound(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	resp := doRequest(t, cfg, http.MethodGet, "/Users/nonexistent-id-that-does-not-exist-xyz", nil)
	checkErrorResponse(t, resp, http.StatusNotFound)
}

func testConflict(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	suffix := uniqueSuffix(t, "dup")

	id, _ := createUser(t, cfg, suffix)
	if id == "" {
		return
	}

	resp := doRequest(t, cfg, http.MethodPost, "/Users", NewTestUser(suffix))

	if resp.StatusCode == http.StatusCreated {

		var body map[string]interface{}
		mustDecodeJSON(t, resp, &body)
		dupID := checkID(t, body)
		if dupID != "" {
			del := doRequest(t, cfg, http.MethodDelete, "/Users/"+dupID, nil)
			drainAndClose(del)
		}
		t.Errorf("expected 409 for duplicate userName, got 201")
		return
	}

	checkErrorResponse(t, resp, http.StatusConflict)
}

func testBadFilter(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	resp := doRequest(t, cfg, http.MethodGet, "/Users?filter=!!!invalid_filter!!!", nil)
	if resp.StatusCode != http.StatusBadRequest {
		drainAndClose(resp)
		t.Fatalf("GET /Users?filter=!!!...: expected 400, got %d", resp.StatusCode)
	}

	var errBody map[string]interface{}
	mustDecodeJSON(t, resp, &errBody)

	checkSchemas(t, errBody, "urn:ietf:params:scim:api:messages:2.0:Error")

	if _, ok := errBody["status"]; !ok {
		t.Errorf("SCIM error response missing 'status' field")
	}

	scimType, _ := errBody["scimType"].(string)
	if scimType != "" && scimType != "invalidFilter" {
		t.Logf("testBadFilter: scimType=%q, expected 'invalidFilter' (non-fatal)", scimType)
	}
}

func testContentType(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	resp := doRequest(t, cfg, http.MethodGet, "/ServiceProviderConfig", nil)
	drainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ServiceProviderConfig: expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		t.Errorf("GET /ServiceProviderConfig: missing Content-Type header")
	}

	req2Body := doRequest(t, cfg, http.MethodGet, "/ServiceProviderConfig", nil)
	req2Body.Request.Header.Set("Accept", "application/scim+json")
	drainAndClose(req2Body)

	t.Run("AcceptJSON", func(t *testing.T) {
		resp3 := doRequestWithAccept(t, cfg, http.MethodGet, "/ServiceProviderConfig", "application/json")
		defer drainAndClose(resp3)
		if resp3.StatusCode == http.StatusNotAcceptable {
			t.Errorf("server rejected Accept: application/json with 406; RFC 7644 §3.8 requires support")
		}
	})
}

func doRequestWithAccept(t *testing.T, cfg SuiteConfig, method, path, accept string) *http.Response {
	t.Helper()
	resp := doRequest(t, cfg, method, path, nil)

	drainAndClose(resp)

	req, err := http.NewRequest(method, cfg.BaseURL+path, nil)
	if err != nil {
		t.Fatalf("doRequestWithAccept: new request: %v", err)
	}
	req.Header.Set("Accept", accept)
	if cfg.AuthHeader != "" {
		req.Header.Set("Authorization", cfg.AuthHeader)
	}
	r, err := cfg.HTTPClient.Do(req)
	if err != nil {
		t.Skipf("compliance: server unreachable: %v", err)
	}
	return r
}
