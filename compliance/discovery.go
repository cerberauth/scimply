package compliance

import (
	"net/http"
	"testing"
)

const (
	serviceProviderConfigSchema = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	schemaSchema                = "urn:ietf:params:scim:schemas:core:2.0:Schema"
	resourceTypeSchema          = "urn:ietf:params:scim:schemas:core:2.0:ResourceType"
	listResponseSchema          = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
)

func testServiceProviderConfig(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	resp := doRequest(t, cfg, http.MethodGet, "/ServiceProviderConfig", nil)
	defer drainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ServiceProviderConfig: expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	mustDecodeJSON(t, resp, &body)

	checkSchemas(t, body, serviceProviderConfigSchema)

	for _, field := range []string{"patch", "bulk", "filter", "changePassword", "sort", "etag", "authenticationSchemes"} {
		if _, ok := body[field]; !ok {
			t.Errorf("ServiceProviderConfig missing field %q", field)
		}
	}
}

func testSchemas(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	resp := doRequest(t, cfg, http.MethodGet, "/Schemas", nil)
	defer drainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /Schemas: expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	mustDecodeJSON(t, resp, &body)

	checkSchemas(t, body, listResponseSchema)

	resources, ok := body["Resources"].([]interface{})
	if !ok || len(resources) == 0 {
		t.Fatalf("GET /Schemas: 'Resources' is empty or missing")
	}

	for i, raw := range resources {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			t.Errorf("Schemas[%d]: not an object", i)
			continue
		}
		checkSchemas(t, entry, schemaSchema)
		if id, _ := entry["id"].(string); id == "" {
			t.Errorf("Schemas[%d]: missing or empty 'id'", i)
		}
	}

	wantIDs := map[string]bool{
		"urn:ietf:params:scim:schemas:core:2.0:User":  false,
		"urn:ietf:params:scim:schemas:core:2.0:Group": false,
	}
	for _, raw := range resources {
		entry, _ := raw.(map[string]interface{})
		id, _ := entry["id"].(string)
		if _, wanted := wantIDs[id]; wanted {
			wantIDs[id] = true
		}
	}
	for id, found := range wantIDs {
		if !found {
			t.Errorf("GET /Schemas: core schema %q not found in response", id)
		}
	}
}

func testResourceTypes(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	resp := doRequest(t, cfg, http.MethodGet, "/ResourceTypes", nil)
	defer drainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ResourceTypes: expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	mustDecodeJSON(t, resp, &body)

	checkSchemas(t, body, listResponseSchema)

	resources, ok := body["Resources"].([]interface{})
	if !ok || len(resources) == 0 {
		t.Fatalf("GET /ResourceTypes: 'Resources' is empty or missing")
	}

	for i, raw := range resources {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			t.Errorf("ResourceTypes[%d]: not an object", i)
			continue
		}
		checkSchemas(t, entry, resourceTypeSchema)
		if name, _ := entry["name"].(string); name == "" {
			t.Errorf("ResourceTypes[%d]: missing or empty 'name'", i)
		}
		if endpoint, _ := entry["endpoint"].(string); endpoint == "" {
			t.Errorf("ResourceTypes[%d]: missing or empty 'endpoint'", i)
		}
	}

	wantNames := map[string]bool{
		"User":  false,
		"Group": false,
	}
	for _, raw := range resources {
		entry, _ := raw.(map[string]interface{})
		name, _ := entry["name"].(string)
		if _, wanted := wantNames[name]; wanted {
			wantNames[name] = true
		}
	}
	for name, found := range wantNames {
		if !found {
			t.Errorf("GET /ResourceTypes: resource type %q not found in response", name)
		}
	}
}
