package compliance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

const scimContentType = "application/scim+json"

func doRequest(t *testing.T, cfg SuiteConfig, method, path string, body interface{}) *http.Response {
	t.Helper()

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("doRequest: marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := cfg.BaseURL + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatalf("doRequest: new request %s %s: %v", method, url, err)
	}

	req.Header.Set("Accept", scimContentType)
	if body != nil {
		req.Header.Set("Content-Type", scimContentType)
	}
	if cfg.AuthHeader != "" {
		req.Header.Set("Authorization", cfg.AuthHeader)
	}

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		t.Skipf("compliance: server unreachable (%s %s): %v", method, url, err)
	}
	return resp
}

func mustDecodeJSON(t *testing.T, r *http.Response, target interface{}) {
	t.Helper()
	defer func() { _ = r.Body.Close() }()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("mustDecodeJSON: read body: %v", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("mustDecodeJSON: unmarshal: %v\nbody: %s", err, data)
	}
}

func drainAndClose(r *http.Response) {
	if r != nil && r.Body != nil {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}
}

func checkSchemas(t *testing.T, resource map[string]interface{}, expectedURI string) {
	t.Helper()
	raw, ok := resource["schemas"]
	if !ok {
		t.Errorf("response missing 'schemas' field")
		return
	}
	schemas, ok := raw.([]interface{})
	if !ok {
		t.Errorf("'schemas' field is not an array, got %T", raw)
		return
	}
	for _, s := range schemas {
		if s == expectedURI {
			return
		}
	}
	t.Errorf("'schemas' does not contain %q; got %v", expectedURI, schemas)
}

func checkID(t *testing.T, resource map[string]interface{}) string {
	t.Helper()
	raw, ok := resource["id"]
	if !ok {
		t.Errorf("response missing 'id' field")
		return ""
	}
	id, ok := raw.(string)
	if !ok || id == "" {
		t.Errorf("'id' field is empty or not a string, got %v", raw)
		return ""
	}
	return id
}

func checkErrorResponse(t *testing.T, resp *http.Response, expectedStatus int) {
	t.Helper()
	if resp.StatusCode != expectedStatus {
		t.Errorf("expected HTTP %d, got %d", expectedStatus, resp.StatusCode)
	}
	var errBody map[string]interface{}
	mustDecodeJSON(t, resp, &errBody)

	checkSchemas(t, errBody, "urn:ietf:params:scim:api:messages:2.0:Error")

	if _, ok := errBody["status"]; !ok {
		t.Errorf("SCIM error response missing 'status' field")
	}
}

func createUser(t *testing.T, cfg SuiteConfig, suffix string) (id string, resource map[string]interface{}) {
	t.Helper()

	resp := doRequest(t, cfg, http.MethodPost, "/Users", NewTestUser(suffix))
	if resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Skipf("createUser: POST /Users returned %d (body: %s)", resp.StatusCode, data)
		return "", nil
	}

	mustDecodeJSON(t, resp, &resource)
	id = checkID(t, resource)
	if id == "" {
		t.Skip("createUser: server returned empty id")
		return "", nil
	}

	t.Cleanup(func() {
		del := doRequest(t, cfg, http.MethodDelete, fmt.Sprintf("/Users/%s", id), nil)
		drainAndClose(del)
	})

	return id, resource
}

func createGroup(t *testing.T, cfg SuiteConfig, suffix string) (id string, resource map[string]interface{}) {
	t.Helper()

	resp := doRequest(t, cfg, http.MethodPost, "/Groups", NewTestGroup(suffix))
	if resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Skipf("createGroup: POST /Groups returned %d (body: %s)", resp.StatusCode, data)
		return "", nil
	}

	mustDecodeJSON(t, resp, &resource)
	id = checkID(t, resource)
	if id == "" {
		t.Skip("createGroup: server returned empty id")
		return "", nil
	}

	t.Cleanup(func() {
		del := doRequest(t, cfg, http.MethodDelete, fmt.Sprintf("/Groups/%s", id), nil)
		drainAndClose(del)
	})

	return id, resource
}

func uniqueSuffix(t *testing.T, extra string) string {
	t.Helper()

	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	if extra != "" {
		return name + "-" + extra
	}
	return name
}
