package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cerberauth/scimply/schema"
	"github.com/cerberauth/scimply/server"
	"github.com/cerberauth/scimply/store"
)

func newTestServer(t *testing.T, opts ...server.Option) *server.Server {
	t.Helper()

	reg := schema.NewRegistry()
	reg.RegisterDefaults()

	ms := store.NewMemoryStore()

	baseOpts := []server.Option{
		server.WithStore(ms),
		server.WithSchemaRegistry(reg),
	}
	baseOpts = append(baseOpts, opts...)

	s, err := server.New(baseOpts...)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return s
}

func doRequest(t *testing.T, s *server.Server, method, path, body string) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/scim+json")
	}
	req.Header.Set("Accept", "application/scim+json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	return rr.Result()
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}
	return string(b)
}

func decodeJSON(t *testing.T, resp *http.Response, dest interface{}) {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("decodeJSON: read body: %v", err)
	}
	if err := json.Unmarshal(b, dest); err != nil {
		t.Fatalf("decodeJSON: unmarshal: %v\nbody: %s", err, b)
	}
}

func TestServerCreate(t *testing.T) {
	s := newTestServer(t)

	body := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"alice"}`
	resp := doRequest(t, s, http.MethodPost, "/Users", body)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d\nbody: %s", resp.StatusCode, readBody(t, resp))
	}

	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	if m["id"] == nil || m["id"] == "" {
		t.Errorf("expected non-empty id, got %v", m["id"])
	}
	if m["userName"] != "alice" {
		t.Errorf("expected userName=alice, got %v", m["userName"])
	}

	if resp.Header.Get("Location") == "" {
		t.Error("expected Location header on 201 response")
	}
}

func TestServerGet(t *testing.T) {
	s := newTestServer(t)

	createBody := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bob"}`
	createResp := doRequest(t, s, http.MethodPost, "/Users", createBody)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	resp := doRequest(t, s, http.MethodGet, "/Users/"+id, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d\nbody: %s", resp.StatusCode, readBody(t, resp))
	}

	var m map[string]interface{}
	decodeJSON(t, resp, &m)
	if m["id"] != id {
		t.Errorf("expected id=%s, got %v", id, m["id"])
	}
	if m["userName"] != "bob" {
		t.Errorf("expected userName=bob, got %v", m["userName"])
	}
}

func TestServerList(t *testing.T) {
	s := newTestServer(t)

	for _, name := range []string{"user1", "user2"} {
		body := fmt.Sprintf(`{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":%q}`, name)
		doRequest(t, s, http.MethodPost, "/Users", body)
	}

	resp := doRequest(t, s, http.MethodGet, "/Users", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d\nbody: %s", resp.StatusCode, readBody(t, resp))
	}

	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)

	totalResults, ok := listResp["totalResults"].(float64)
	if !ok || int(totalResults) < 2 {
		t.Errorf("expected totalResults>=2, got %v", listResp["totalResults"])
	}

	resources, ok := listResp["Resources"].([]interface{})
	if !ok {
		t.Fatalf("expected Resources array, got %T", listResp["Resources"])
	}
	if len(resources) < 2 {
		t.Errorf("expected at least 2 resources, got %d", len(resources))
	}
}

func TestServerListFilter(t *testing.T) {
	s := newTestServer(t)

	for _, name := range []string{"filteruser1", "filteruser2"} {
		body := fmt.Sprintf(`{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":%q}`, name)
		doRequest(t, s, http.MethodPost, "/Users", body)
	}

	resp := doRequest(t, s, http.MethodGet, `/Users?filter=userName+eq+"filteruser1"`, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d\nbody: %s", resp.StatusCode, readBody(t, resp))
	}

	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)

	resources, ok := listResp["Resources"].([]interface{})
	if !ok {
		t.Fatalf("expected Resources array")
	}
	if len(resources) != 1 {
		t.Errorf("expected 1 filtered resource, got %d", len(resources))
	}
	if len(resources) > 0 {
		m := resources[0].(map[string]interface{})
		if m["userName"] != "filteruser1" {
			t.Errorf("expected userName=filteruser1, got %v", m["userName"])
		}
	}
}

func TestServerReplace(t *testing.T) {
	s := newTestServer(t)

	createBody := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"replaceuser"}`
	createResp := doRequest(t, s, http.MethodPost, "/Users", createBody)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	replaceBody := fmt.Sprintf(`{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"replaceuser","displayName":"Alice Updated","id":%q}`, id)
	resp := doRequest(t, s, http.MethodPut, "/Users/"+id, replaceBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d\nbody: %s", resp.StatusCode, readBody(t, resp))
	}

	var m map[string]interface{}
	decodeJSON(t, resp, &m)
	if m["displayName"] != "Alice Updated" {
		t.Errorf("expected displayName=Alice Updated, got %v", m["displayName"])
	}
}

func TestServerPatch(t *testing.T) {
	s := newTestServer(t)

	createBody := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"patchuser"}`
	createResp := doRequest(t, s, http.MethodPost, "/Users", createBody)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	patchBody := `{
		"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
		"Operations":[{"op":"add","path":"displayName","value":"Patched Name"}]
	}`
	resp := doRequest(t, s, http.MethodPatch, "/Users/"+id, patchBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d\nbody: %s", resp.StatusCode, readBody(t, resp))
	}

	var m map[string]interface{}
	decodeJSON(t, resp, &m)
	if m["displayName"] != "Patched Name" {
		t.Errorf("expected displayName=Patched Name, got %v", m["displayName"])
	}
}

func TestServerDelete(t *testing.T) {
	s := newTestServer(t)

	createBody := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"deleteuser"}`
	createResp := doRequest(t, s, http.MethodPost, "/Users", createBody)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	resp := doRequest(t, s, http.MethodDelete, "/Users/"+id, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d\nbody: %s", resp.StatusCode, readBody(t, resp))
	}

	getResp := doRequest(t, s, http.MethodGet, "/Users/"+id, "")
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", getResp.StatusCode)
	}
}

func TestServerNotFound(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, "/Users/nonexistent-id", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}

	var m map[string]interface{}
	decodeJSON(t, resp, &m)
	if schemas, ok := m["schemas"].([]interface{}); !ok || len(schemas) == 0 {
		t.Error("expected SCIM error response with schemas field")
	}
}

func TestServerConflict(t *testing.T) {
	s := newTestServer(t)

	body := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"conflictuser"}`
	first := doRequest(t, s, http.MethodPost, "/Users", body)
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 on first create, got %d", first.StatusCode)
	}
	_, _ = io.Copy(io.Discard, first.Body)

	second := doRequest(t, s, http.MethodPost, "/Users", body)
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate create, got %d\nbody: %s", second.StatusCode, readBody(t, second))
	}

	var m map[string]interface{}
	decodeJSON(t, second, &m)
	if m["scimType"] != "uniqueness" {
		t.Errorf("expected scimType=uniqueness, got %v", m["scimType"])
	}
}

func TestServerDiscovery(t *testing.T) {
	s := newTestServer(t)

	t.Run("ServiceProviderConfig", func(t *testing.T) {
		resp := doRequest(t, s, http.MethodGet, "/ServiceProviderConfig", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d\nbody: %s", resp.StatusCode, readBody(t, resp))
		}
		var m map[string]interface{}
		decodeJSON(t, resp, &m)
		schemas, ok := m["schemas"].([]interface{})
		if !ok || len(schemas) == 0 {
			t.Error("expected schemas field in ServiceProviderConfig response")
		} else if schemas[0] != "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig" {
			t.Errorf("unexpected schema URI: %v", schemas[0])
		}
	})

	t.Run("ResourceTypes", func(t *testing.T) {
		resp := doRequest(t, s, http.MethodGet, "/ResourceTypes", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d\nbody: %s", resp.StatusCode, readBody(t, resp))
		}
		var m map[string]interface{}
		decodeJSON(t, resp, &m)
		if m["totalResults"] == nil {
			t.Error("expected totalResults in ResourceTypes list response")
		}
	})

	t.Run("ResourceTypeByName", func(t *testing.T) {
		resp := doRequest(t, s, http.MethodGet, "/ResourceTypes/User", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d\nbody: %s", resp.StatusCode, readBody(t, resp))
		}
		var m map[string]interface{}
		decodeJSON(t, resp, &m)
		if m["name"] != "User" {
			t.Errorf("expected name=User, got %v", m["name"])
		}
	})

	t.Run("Schemas", func(t *testing.T) {
		resp := doRequest(t, s, http.MethodGet, "/Schemas", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d\nbody: %s", resp.StatusCode, readBody(t, resp))
		}
		var m map[string]interface{}
		decodeJSON(t, resp, &m)
		if m["totalResults"] == nil {
			t.Error("expected totalResults in Schemas list response")
		}
	})

	t.Run("SchemaByID", func(t *testing.T) {
		resp := doRequest(t, s, http.MethodGet, "/Schemas/urn:ietf:params:scim:schemas:core:2.0:User", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d\nbody: %s", resp.StatusCode, readBody(t, resp))
		}
		var m map[string]interface{}
		decodeJSON(t, resp, &m)
		if m["id"] != "urn:ietf:params:scim:schemas:core:2.0:User" {
			t.Errorf("expected id=urn:...:User, got %v", m["id"])
		}
	})
}

func TestServerBulk(t *testing.T) {
	s := newTestServer(t)

	bulkBody := `{
		"schemas":["urn:ietf:params:scim:api:messages:2.0:BulkRequest"],
		"failOnErrors":1,
		"Operations":[
			{
				"method":"POST",
				"path":"/Users",
				"bulkId":"bulkop1",
				"data":{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bulkuser1"}
			},
			{
				"method":"POST",
				"path":"/Users",
				"bulkId":"bulkop2",
				"data":{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bulkuser2"}
			}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/Bulk", bytes.NewBufferString(bulkBody))
	req.Header.Set("Content-Type", "application/scim+json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	resp := rr.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d\nbody: %s", resp.StatusCode, readBody(t, resp))
	}

	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	ops, ok := m["Operations"].([]interface{})
	if !ok || len(ops) != 2 {
		t.Fatalf("expected 2 Operations in bulk response, got %v", m["Operations"])
	}

	for i, opRaw := range ops {
		op := opRaw.(map[string]interface{})
		status := op["status"].(string)
		if status != "201" {
			t.Errorf("op %d: expected status=201, got %s", i, status)
		}
	}
}

func TestServerAuth(t *testing.T) {
	validToken := "secret-token"
	s := newTestServer(t, server.WithBearerTokenAuth(func(token string) (bool, error) {
		return token == validToken, nil
	}))

	t.Run("missing auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/Users", nil)
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("wrong token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/Users", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("valid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/Users", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
	})
}

func TestServerContentType(t *testing.T) {
	s := newTestServer(t)

	t.Run("unsupported content-type rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/Users", strings.NewReader(`{"userName":"ct-user"}`))
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnsupportedMediaType {
			t.Errorf("expected 415, got %d", rr.Code)
		}
	})

	t.Run("application/json accepted", func(t *testing.T) {
		body := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"ct-user-json"}`
		req := httptest.NewRequest(http.MethodPost, "/Users", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("response content-type is scim+json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/Users", nil)
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		ct := rr.Header().Get("Content-Type")
		if !strings.HasPrefix(ct, "application/scim+json") {
			t.Errorf("expected application/scim+json response Content-Type, got %s", ct)
		}
	})
}
