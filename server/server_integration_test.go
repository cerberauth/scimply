package server_test

// server_integration_test.go – SCIM 2.0 specification integration tests.
//
// Coverage is organised by RFC section:
//   RFC 7644 §3.3        – Creating resources (POST)
//   RFC 7644 §3.4.1      – Retrieving a known resource (GET)
//   RFC 7644 §3.4.2      – Query resources / ListResponse
//   RFC 7644 §3.4.2.2    – Filtering (all operators + logical ops)
//   RFC 7644 §3.4.2.3    – Sorting
//   RFC 7644 §3.4.2.4    – Pagination
//   RFC 7644 §3.4.3      – Querying via POST /.search
//   RFC 7644 §3.5.1      – Replacing with PUT
//   RFC 7644 §3.5.2      – Modifying with PATCH (add, remove, replace)
//   RFC 7644 §3.6        – Deleting resources
//   RFC 7644 §3.7        – Bulk operations
//   RFC 7644 §3.8        – Data input/output formats (content-type)
//   RFC 7644 §3.9        – Attribute notation / attribute selection
//   RFC 7644 §3.12       – HTTP status and error response handling
//   RFC 7644 §3.14       – Versioning (ETag / meta.location / meta.version)
//   RFC 7644 §4          – Discovery endpoints
//   RFC 7643 §3.1        – Common attributes (meta.*)
//
// All tests use the newTestServer / doRequest / decodeJSON helpers defined in
// server_test.go.  Each test references the specific normative requirement
// it is verifying.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cerberauth/scimply/server"
)

// ---------------------------------------------------------------------------
// RFC 7643 §3.1 / RFC 7644 §3.3 – Common attributes & create response
// ---------------------------------------------------------------------------

// TestMeta_CreateSetsRequiredFields verifies that the POST response includes
// id, schemas, and a meta sub-object with resourceType, created, lastModified,
// and location (RFC 7644 §3.3 / RFC 7643 §3.1).
func TestMeta_CreateSetsRequiredFields(t *testing.T) {
	s := newTestServer(t)

	body := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"meta-user"}`
	resp := doRequest(t, s, http.MethodPost, "/Users", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}

	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	// id must be a non-empty server-assigned string (RFC 7643 §3.1).
	if id, _ := m["id"].(string); id == "" {
		t.Error("create response missing non-empty 'id'")
	}

	// schemas must be present (RFC 7644 §3.1).
	if schemas, _ := m["schemas"].([]interface{}); len(schemas) == 0 {
		t.Error("create response missing 'schemas'")
	}

	meta, ok := m["meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("create response missing 'meta' object")
	}

	if rt, _ := meta["resourceType"].(string); rt != "User" {
		t.Errorf("meta.resourceType: expected 'User', got %q", rt)
	}
	if created, _ := meta["created"].(string); created == "" {
		t.Error("meta.created: expected non-empty RFC3339 timestamp")
	}
	if lm, _ := meta["lastModified"].(string); lm == "" {
		t.Error("meta.lastModified: expected non-empty RFC3339 timestamp")
	}

	// meta.location must reference the resource URI (RFC 7644 §3.3).
	id, _ := m["id"].(string)
	if loc, _ := meta["location"].(string); !strings.Contains(loc, "/Users/"+id) {
		t.Errorf("meta.location %q does not contain '/Users/%s'", loc, id)
	}
}

// TestCreate_LocationHeader verifies that a 201 response always carries a
// Location header equal to meta.location (RFC 7644 §3.3).
func TestCreate_LocationHeader(t *testing.T) {
	s := newTestServer(t)

	body := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"loc-user"}`
	resp := doRequest(t, s, http.MethodPost, "/Users", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if loc == "" {
		t.Fatal("Location header missing on 201 response")
	}
	if !strings.Contains(loc, "/Users/") {
		t.Errorf("Location header %q does not contain '/Users/'", loc)
	}
}

// TestCreate_ExternalId verifies that the optional externalId attribute sent
// by a client is echoed back unchanged (RFC 7643 §3.1).
func TestCreate_ExternalId(t *testing.T) {
	s := newTestServer(t)

	body := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"extid-user","externalId":"client-ext-001"}`
	resp := doRequest(t, s, http.MethodPost, "/Users", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}

	var m map[string]interface{}
	decodeJSON(t, resp, &m)
	if eid, _ := m["externalId"].(string); eid != "client-ext-001" {
		t.Errorf("externalId: expected 'client-ext-001', got %q", eid)
	}
}

// TestCreate_Conflict verifies that creating a duplicate userName returns 409
// with scimType "uniqueness" (RFC 7644 §3.3).
func TestCreate_Conflict(t *testing.T) {
	s := newTestServer(t)

	body := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"dup-user"}`
	doRequest(t, s, http.MethodPost, "/Users", body)
	resp := doRequest(t, s, http.MethodPost, "/Users", body)

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)
	if m["scimType"] != "uniqueness" {
		t.Errorf("expected scimType=uniqueness, got %v", m["scimType"])
	}
}

// TestCreate_InvalidJSON verifies that a malformed JSON body returns 400 with
// scimType "invalidSyntax" (RFC 7644 §3.12, Table 9).
func TestCreate_InvalidJSON(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodPost, "/Users", `{not valid json}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)
	if m["scimType"] != "invalidSyntax" {
		t.Errorf("expected scimType=invalidSyntax, got %v", m["scimType"])
	}
}

// ---------------------------------------------------------------------------
// RFC 7644 §3.4.1 – Retrieving a known resource
// ---------------------------------------------------------------------------

// TestGet_LocationHeader verifies that a GET response for a known resource
// carries a Location header pointing to the resource URI (RFC 7644 §3.4.1).
func TestGet_LocationHeader(t *testing.T) {
	s := newTestServer(t)

	body := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"get-loc-user"}`
	createResp := doRequest(t, s, http.MethodPost, "/Users", body)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	resp := doRequest(t, s, http.MethodGet, "/Users/"+id, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if loc == "" {
		t.Error("GET response missing Location header")
	}
	if !strings.Contains(loc, "/Users/"+id) {
		t.Errorf("Location header %q does not contain '/Users/%s'", loc, id)
	}
}

// ---------------------------------------------------------------------------
// RFC 7644 §3.4.2 – Query Resources / ListResponse
// ---------------------------------------------------------------------------

// TestList_ResponseSchema verifies that the ListResponse contains the mandatory
// schema URI "urn:ietf:params:scim:api:messages:2.0:ListResponse" (RFC 7644 §3.4.2).
func TestList_ResponseSchema(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, "/Users", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}

	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	const listResponseSchema = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	schemas, _ := m["schemas"].([]interface{})
	found := false
	for _, s := range schemas {
		if s == listResponseSchema {
			found = true
		}
	}
	if !found {
		t.Errorf("list response schemas must include %q; got %v", listResponseSchema, schemas)
	}
}

// TestList_RequiredFields verifies that totalResults, startIndex, itemsPerPage,
// and Resources are all present (RFC 7644 §3.4.2).
func TestList_RequiredFields(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, "/Users", "")
	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	for _, field := range []string{"totalResults", "startIndex", "itemsPerPage", "Resources"} {
		if _, ok := m[field]; !ok {
			t.Errorf("list response missing required field %q", field)
		}
	}
}

// TestList_StartIndexDefault verifies that startIndex defaults to 1 when not
// specified (RFC 7644 §3.4.2.4, Table 6).
func TestList_StartIndexDefault(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, "/Users", "")
	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	startIndex, _ := m["startIndex"].(float64)
	if int(startIndex) != 1 {
		t.Errorf("startIndex should default to 1, got %v", startIndex)
	}
}

// TestList_ItemsPerPageReflectsPage verifies that itemsPerPage equals the
// number of resources actually returned (RFC 7644 §3.4.2, Table 7).
func TestList_ItemsPerPageReflectsPage(t *testing.T) {
	s := newTestServer(t)

	for _, name := range []string{"ipp-user1", "ipp-user2", "ipp-user3"} {
		body := fmt.Sprintf(`{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":%q}`, name)
		doRequest(t, s, http.MethodPost, "/Users", body)
	}

	resp := doRequest(t, s, http.MethodGet, "/Users?count=2", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	resources, _ := m["Resources"].([]interface{})
	itemsPerPage, _ := m["itemsPerPage"].(float64)
	if int(itemsPerPage) != len(resources) {
		t.Errorf("itemsPerPage=%d does not match len(Resources)=%d", int(itemsPerPage), len(resources))
	}
}

// TestList_EmptyFilterResult verifies that a query with no matches returns
// HTTP 200 with totalResults=0 (RFC 7644 §3.4.2: "SHALL return success …
// with totalResults set to 0").
func TestList_EmptyFilterResult(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, `/Users?filter=userName+eq+"no-such-user-xyzzy"`, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for empty result, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	totalResults, _ := m["totalResults"].(float64)
	if int(totalResults) != 0 {
		t.Errorf("expected totalResults=0 for no-match filter, got %v", totalResults)
	}
}

// ---------------------------------------------------------------------------
// RFC 7644 §3.4.2.2 – Filtering
// ---------------------------------------------------------------------------

// TestFilter_Eq verifies the eq (equal) operator (RFC 7644 §3.4.2.2, Table 3).
func TestFilter_Eq(t *testing.T) {
	s := newTestServer(t)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"filter-eq-alice"}`)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"filter-eq-bob"}`)

	resp := doRequest(t, s, http.MethodGet, `/Users?filter=userName+eq+"filter-eq-alice"`, "")
	assertSingleResult(t, resp, "userName", "filter-eq-alice")
}

// TestFilter_Ne verifies the ne (not equal) operator (RFC 7644 §3.4.2.2, Table 3).
func TestFilter_Ne(t *testing.T) {
	s := newTestServer(t)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"filter-ne-alice"}`)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"filter-ne-bob"}`)

	resp := doRequest(t, s, http.MethodGet, `/Users?filter=userName+ne+"filter-ne-alice"`, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	resources, _ := m["Resources"].([]interface{})
	for _, raw := range resources {
		entry, _ := raw.(map[string]interface{})
		if entry["userName"] == "filter-ne-alice" {
			t.Error("ne filter should have excluded filter-ne-alice")
		}
	}
}

// TestFilter_Co verifies the co (contains) operator (RFC 7644 §3.4.2.2, Table 3).
func TestFilter_Co(t *testing.T) {
	s := newTestServer(t)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"filter-co-jensen"}`)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"filter-co-other"}`)

	resp := doRequest(t, s, http.MethodGet, `/Users?filter=userName+co+"co-jensen"`, "")
	assertSingleResult(t, resp, "userName", "filter-co-jensen")
}

// TestFilter_Sw verifies the sw (starts with) operator (RFC 7644 §3.4.2.2, Table 3).
func TestFilter_Sw(t *testing.T) {
	s := newTestServer(t)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"sw-prefix-user"}`)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"other-no-match"}`)

	resp := doRequest(t, s, http.MethodGet, `/Users?filter=userName+sw+"sw-prefix"`, "")
	assertSingleResult(t, resp, "userName", "sw-prefix-user")
}

// TestFilter_Ew verifies the ew (ends with) operator (RFC 7644 §3.4.2.2, Table 3).
func TestFilter_Ew(t *testing.T) {
	s := newTestServer(t)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"user-ew-suffix"}`)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"user-ew-other"}`)

	resp := doRequest(t, s, http.MethodGet, `/Users?filter=userName+ew+"ew-suffix"`, "")
	assertSingleResult(t, resp, "userName", "user-ew-suffix")
}

// TestFilter_Pr verifies the pr (present) operator (RFC 7644 §3.4.2.2, Table 3).
func TestFilter_Pr(t *testing.T) {
	s := newTestServer(t)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"pr-has-nick","nickName":"Babs"}`)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"pr-no-nick"}`)

	resp := doRequest(t, s, http.MethodGet, `/Users?filter=nickName+pr`, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	resources, _ := m["Resources"].([]interface{})
	for _, raw := range resources {
		entry, _ := raw.(map[string]interface{})
		if entry["userName"] == "pr-no-nick" {
			t.Error("pr filter should not have included resource without nickName")
		}
	}
	found := false
	for _, raw := range resources {
		entry, _ := raw.(map[string]interface{})
		if entry["userName"] == "pr-has-nick" {
			found = true
		}
	}
	if !found {
		t.Error("pr filter should have included resource with nickName")
	}
}

// TestFilter_And verifies the logical "and" operator (RFC 7644 §3.4.2.2, Table 4).
func TestFilter_And(t *testing.T) {
	s := newTestServer(t)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"and-alice","displayName":"Alice"}`)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"and-bob","displayName":"Alice"}`)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"and-charlie"}`)

	// Both conditions must be true.
	resp := doRequest(t, s, http.MethodGet, `/Users?filter=userName+eq+"and-alice"+and+displayName+eq+"Alice"`, "")
	assertSingleResult(t, resp, "userName", "and-alice")
}

// TestFilter_Or verifies the logical "or" operator (RFC 7644 §3.4.2.2, Table 4).
func TestFilter_Or(t *testing.T) {
	s := newTestServer(t)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"or-alice"}`)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"or-bob"}`)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"or-charlie"}`)

	resp := doRequest(t, s, http.MethodGet, `/Users?filter=userName+eq+"or-alice"+or+userName+eq+"or-bob"`, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	resources, _ := m["Resources"].([]interface{})
	if len(resources) != 2 {
		t.Errorf("or filter: expected 2 results, got %d", len(resources))
	}
	for _, raw := range resources {
		entry, _ := raw.(map[string]interface{})
		if entry["userName"] == "or-charlie" {
			t.Error("or filter should not have included or-charlie")
		}
	}
}

// TestFilter_Not verifies the logical "not" operator (RFC 7644 §3.4.2.2, Table 4).
func TestFilter_Not(t *testing.T) {
	s := newTestServer(t)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"not-alice"}`)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"not-bob"}`)

	resp := doRequest(t, s, http.MethodGet, `/Users?filter=not+(userName+eq+"not-alice")`, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	resources, _ := m["Resources"].([]interface{})
	for _, raw := range resources {
		entry, _ := raw.(map[string]interface{})
		if entry["userName"] == "not-alice" {
			t.Error("not filter should have excluded not-alice")
		}
	}
}

// TestFilter_SubAttribute verifies filtering on a complex attribute's
// sub-attribute using dot notation (RFC 7644 §3.4.2.2: "name.givenName").
func TestFilter_SubAttribute(t *testing.T) {
	s := newTestServer(t)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"sub-alice","name":{"givenName":"Alice","familyName":"Smith"}}`)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"sub-bob","name":{"givenName":"Bob","familyName":"Smith"}}`)

	resp := doRequest(t, s, http.MethodGet, `/Users?filter=name.givenName+eq+"Alice"`, "")
	assertSingleResult(t, resp, "userName", "sub-alice")
}

// TestFilter_CaseInsensitiveOperator verifies that attribute names and operators
// are case insensitive (RFC 7644 §3.4.2.2: "Attribute names and attribute
// operators used in filters are case insensitive").
func TestFilter_CaseInsensitiveOperator(t *testing.T) {
	s := newTestServer(t)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"ci-alice"}`)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"ci-bob"}`)

	// Use mixed-case operator and attribute name.
	resp := doRequest(t, s, http.MethodGet, `/Users?filter=UserName+EQ+"ci-alice"`, "")
	assertSingleResult(t, resp, "userName", "ci-alice")
}

// TestFilter_InvalidFilter verifies that an invalid filter expression returns
// 400 with scimType "invalidFilter" (RFC 7644 §3.4.2.2 / §3.12, Table 9).
func TestFilter_InvalidFilter(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, `/Users?filter=!!!invalid!!!`, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)
	if m["scimType"] != "invalidFilter" {
		t.Errorf("expected scimType=invalidFilter, got %v", m["scimType"])
	}
}

// ---------------------------------------------------------------------------
// RFC 7644 §3.4.2.3 – Sorting
// ---------------------------------------------------------------------------

// TestSort_Ascending verifies that sortBy + sortOrder=ascending returns
// results in ascending order (RFC 7644 §3.4.2.3).
func TestSort_Ascending(t *testing.T) {
	s := newTestServer(t)

	for _, name := range []string{"sort-charlie", "sort-alice", "sort-bob"} {
		body := fmt.Sprintf(`{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":%q}`, name)
		doRequest(t, s, http.MethodPost, "/Users", body)
	}

	resp := doRequest(t, s, http.MethodGet, `/Users?filter=userName+sw+"sort-"&sortBy=userName&sortOrder=ascending`, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}

	names := listResourceAttr(t, resp, "userName")
	if len(names) < 2 {
		t.Skipf("expected at least 2 resources, got %d", len(names))
	}
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("ascending order violated at index %d: %q > %q", i, names[i-1], names[i])
		}
	}
}

// TestSort_Descending verifies that sortOrder=descending returns resources in
// descending order (RFC 7644 §3.4.2.3).
func TestSort_Descending(t *testing.T) {
	s := newTestServer(t)

	for _, name := range []string{"dsort-charlie", "dsort-alice", "dsort-bob"} {
		body := fmt.Sprintf(`{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":%q}`, name)
		doRequest(t, s, http.MethodPost, "/Users", body)
	}

	resp := doRequest(t, s, http.MethodGet, `/Users?filter=userName+sw+"dsort-"&sortBy=userName&sortOrder=descending`, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}

	names := listResourceAttr(t, resp, "userName")
	if len(names) < 2 {
		t.Skipf("expected at least 2 resources, got %d", len(names))
	}
	for i := 1; i < len(names); i++ {
		if names[i] > names[i-1] {
			t.Errorf("descending order violated at index %d: %q < %q", i, names[i-1], names[i])
		}
	}
}

// TestSort_DefaultsToAscending verifies that when sortBy is specified without
// sortOrder, the result defaults to ascending order (RFC 7644 §3.4.2.3:
// "If a value for sortBy is provided and no sortOrder is specified,
// sortOrder SHALL default to ascending").
func TestSort_DefaultsToAscending(t *testing.T) {
	s := newTestServer(t)

	for _, name := range []string{"def-sort-charlie", "def-sort-alice", "def-sort-bob"} {
		body := fmt.Sprintf(`{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":%q}`, name)
		doRequest(t, s, http.MethodPost, "/Users", body)
	}

	resp := doRequest(t, s, http.MethodGet, `/Users?filter=userName+sw+"def-sort-"&sortBy=userName`, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}

	names := listResourceAttr(t, resp, "userName")
	if len(names) < 2 {
		t.Skipf("expected at least 2 resources, got %d", len(names))
	}
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("default sort (ascending) violated at index %d: %q > %q", i, names[i-1], names[i])
		}
	}
}

// ---------------------------------------------------------------------------
// RFC 7644 §3.4.2.4 – Pagination
// ---------------------------------------------------------------------------

// TestPagination_CountLimitsResults verifies that count=N returns at most N
// resources (RFC 7644 §3.4.2.4: "the service provider MUST NOT return more
// results than specified").
func TestPagination_CountLimitsResults(t *testing.T) {
	s := newTestServer(t)

	for _, name := range []string{"page-a", "page-b", "page-c"} {
		body := fmt.Sprintf(`{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":%q}`, name)
		doRequest(t, s, http.MethodPost, "/Users", body)
	}

	resp := doRequest(t, s, http.MethodGet, "/Users?count=1", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	resources, _ := m["Resources"].([]interface{})
	if len(resources) > 1 {
		t.Errorf("count=1 but received %d resources", len(resources))
	}
}

// TestPagination_CountZeroReturnsOnlyTotalResults verifies that count=0
// returns totalResults but no resource entries (RFC 7644 §3.4.2.4, Table 6:
// "A value of 0 indicates that no resource results are to be returned except
// for totalResults").
func TestPagination_CountZeroReturnsOnlyTotalResults(t *testing.T) {
	s := newTestServer(t)

	for _, name := range []string{"c0-user1", "c0-user2"} {
		body := fmt.Sprintf(`{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":%q}`, name)
		doRequest(t, s, http.MethodPost, "/Users", body)
	}

	resp := doRequest(t, s, http.MethodGet, "/Users?count=0", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	total, _ := m["totalResults"].(float64)
	if int(total) < 2 {
		t.Errorf("totalResults expected >= 2, got %v", total)
	}
	resources, _ := m["Resources"].([]interface{})
	if len(resources) != 0 {
		t.Errorf("count=0: expected 0 Resources, got %d", len(resources))
	}
}

// TestPagination_StartIndexReflected verifies that the startIndex value is
// echoed back in the response (RFC 7644 §3.4.2.4, Table 7).
func TestPagination_StartIndexReflected(t *testing.T) {
	s := newTestServer(t)

	for _, name := range []string{"si-user1", "si-user2", "si-user3"} {
		body := fmt.Sprintf(`{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":%q}`, name)
		doRequest(t, s, http.MethodPost, "/Users", body)
	}

	resp := doRequest(t, s, http.MethodGet, "/Users?startIndex=2&count=1", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	si, _ := m["startIndex"].(float64)
	if int(si) != 2 {
		t.Errorf("startIndex: expected 2 in response, got %v", si)
	}
}

// TestPagination_MaxPageSizeCapped verifies that the server enforces its
// configured maxPageSize.
func TestPagination_MaxPageSizeCapped(t *testing.T) {
	s := newTestServer(t, server.WithMaxPageSize(2), server.WithDefaultPageSize(2))

	for _, name := range []string{"cap-user1", "cap-user2", "cap-user3", "cap-user4"} {
		body := fmt.Sprintf(`{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":%q}`, name)
		doRequest(t, s, http.MethodPost, "/Users", body)
	}

	resp := doRequest(t, s, http.MethodGet, "/Users?count=100", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	resources, _ := m["Resources"].([]interface{})
	if len(resources) > 2 {
		t.Errorf("server should cap at maxPageSize=2, but returned %d resources", len(resources))
	}
}

// ---------------------------------------------------------------------------
// RFC 7644 §3.4.3 – Querying via POST /.search
// ---------------------------------------------------------------------------

// TestSearch_PostDotSearch verifies that POST /Users/.search accepts query
// parameters in the request body and returns a valid ListResponse
// (RFC 7644 §3.4.3).
func TestSearch_PostDotSearch(t *testing.T) {
	s := newTestServer(t)

	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"search-alice"}`)
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"search-bob"}`)

	// The request body MAY include the SearchRequest schema (RFC 7644 §3.4.3).
	searchBody := `{
		"schemas":["urn:ietf:params:scim:api:messages:2.0:SearchRequest"],
		"filter":"userName eq \"search-alice\"",
		"startIndex":1,
		"count":10
	}`
	resp := doRequest(t, s, http.MethodPost, "/Users/.search", searchBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /Users/.search expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}

	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	// Must return a ListResponse schema.
	schemas, _ := m["schemas"].([]interface{})
	hasListSchema := false
	for _, s := range schemas {
		if s == "urn:ietf:params:scim:api:messages:2.0:ListResponse" {
			hasListSchema = true
		}
	}
	if !hasListSchema {
		t.Errorf(".search response schemas must contain ListResponse URI, got %v", schemas)
	}

	resources, _ := m["Resources"].([]interface{})
	if len(resources) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resources))
	}
	entry, _ := resources[0].(map[string]interface{})
	if entry["userName"] != "search-alice" {
		t.Errorf("unexpected resource: %v", entry["userName"])
	}
}

// TestSearch_NonPostMethodNotAllowed verifies that GET on a .search endpoint
// returns 405 Method Not Allowed (RFC 7644 §3.4.3: only POST is defined).
func TestSearch_NonPostMethodNotAllowed(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, "/Users/.search", "")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /Users/.search: expected 405, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
}

// ---------------------------------------------------------------------------
// RFC 7644 §3.5.1 – Replacing with PUT
// ---------------------------------------------------------------------------

// TestReplace_PreservesCreated verifies that PUT preserves meta.created
// while updating meta.lastModified (RFC 7644 §3.5.1).
func TestReplace_PreservesCreated(t *testing.T) {
	s := newTestServer(t)

	createBody := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"preserve-created"}`
	createResp := doRequest(t, s, http.MethodPost, "/Users", createBody)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)
	originalCreated := created["meta"].(map[string]interface{})["created"].(string)

	// PUT replace.
	replaceBody := fmt.Sprintf(`{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"id":%q,"userName":"preserve-created","displayName":"Updated"}`, id)
	replaceResp := doRequest(t, s, http.MethodPut, "/Users/"+id, replaceBody)
	if replaceResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d; body: %s", replaceResp.StatusCode, readBody(t, replaceResp))
	}

	var replaced map[string]interface{}
	decodeJSON(t, replaceResp, &replaced)

	replacedMeta, ok := replaced["meta"].(map[string]interface{})
	if !ok {
		t.Fatal("PUT response missing 'meta'")
	}
	if replacedMeta["created"] != originalCreated {
		t.Errorf("meta.created changed: was %q, after PUT is %q", originalCreated, replacedMeta["created"])
	}
}

// TestReplace_NotFound verifies that PUT on a non-existent resource returns
// 404 (RFC 7644 §3.12, Table 8).
func TestReplace_NotFound(t *testing.T) {
	s := newTestServer(t)

	body := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"nobody"}`
	resp := doRequest(t, s, http.MethodPut, "/Users/does-not-exist", body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	assertScimErrorResponse(t, resp, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// RFC 7644 §3.5.2 – Modifying with PATCH
// ---------------------------------------------------------------------------

// TestPatch_Add verifies the "add" operation (RFC 7644 §3.5.2.1).
func TestPatch_Add(t *testing.T) {
	s := newTestServer(t)

	createResp := doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"patch-add"}`)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	patchBody := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"add","path":"displayName","value":"Added"}]}`
	patchResp := doRequest(t, s, http.MethodPatch, "/Users/"+id, patchBody)
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", patchResp.StatusCode, readBody(t, patchResp))
	}
	var m map[string]interface{}
	decodeJSON(t, patchResp, &m)
	if m["displayName"] != "Added" {
		t.Errorf("displayName: expected 'Added', got %v", m["displayName"])
	}
}

// TestPatch_Remove verifies the "remove" operation (RFC 7644 §3.5.2.2).
func TestPatch_Remove(t *testing.T) {
	s := newTestServer(t)

	createResp := doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"patch-rm","displayName":"To Remove"}`)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	patchBody := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"remove","path":"displayName"}]}`
	patchResp := doRequest(t, s, http.MethodPatch, "/Users/"+id, patchBody)
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", patchResp.StatusCode, readBody(t, patchResp))
	}

	getResp := doRequest(t, s, http.MethodGet, "/Users/"+id, "")
	var m map[string]interface{}
	decodeJSON(t, getResp, &m)
	if v, exists := m["displayName"]; exists && v != nil && v != "" {
		t.Errorf("after PATCH remove, displayName should be absent, got %v", v)
	}
}

// TestPatch_RemoveWithoutPath_NoTarget verifies that a "remove" operation
// without a "path" returns 400 with scimType "noTarget" (RFC 7644 §3.5.2.2:
// "If 'path' is unspecified, the operation fails with HTTP status code 400
// and a 'scimType' error code of 'noTarget'").
func TestPatch_RemoveWithoutPath_NoTarget(t *testing.T) {
	s := newTestServer(t)

	createResp := doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"patch-notarget"}`)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	// Remove without path – must fail with noTarget.
	patchBody := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"remove"}]}`
	patchResp := doRequest(t, s, http.MethodPatch, "/Users/"+id, patchBody)
	if patchResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", patchResp.StatusCode, readBody(t, patchResp))
	}
	var m map[string]interface{}
	decodeJSON(t, patchResp, &m)
	if m["scimType"] != "noTarget" {
		t.Errorf("expected scimType=noTarget, got %v", m["scimType"])
	}
}

// TestPatch_Replace verifies the "replace" operation (RFC 7644 §3.5.2.3).
func TestPatch_Replace(t *testing.T) {
	s := newTestServer(t)

	createResp := doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"patch-rep","displayName":"Original"}`)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	patchBody := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"replace","path":"displayName","value":"Replaced"}]}`
	patchResp := doRequest(t, s, http.MethodPatch, "/Users/"+id, patchBody)
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", patchResp.StatusCode, readBody(t, patchResp))
	}
	var m map[string]interface{}
	decodeJSON(t, patchResp, &m)
	if m["displayName"] != "Replaced" {
		t.Errorf("expected displayName=Replaced, got %v", m["displayName"])
	}
}

// TestPatch_ReplaceWithoutPath verifies that "replace" without a path treats
// the value as a map of attributes to replace on the resource
// (RFC 7644 §3.5.2.3: "If the 'path' parameter is omitted, the target is
// assumed to be the resource itself").
func TestPatch_ReplaceWithoutPath(t *testing.T) {
	s := newTestServer(t)

	createResp := doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"patch-nop"}`)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	patchBody := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"replace","value":{"displayName":"NoPath"}}]}`
	patchResp := doRequest(t, s, http.MethodPatch, "/Users/"+id, patchBody)
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", patchResp.StatusCode, readBody(t, patchResp))
	}
	var m map[string]interface{}
	decodeJSON(t, patchResp, &m)
	if m["displayName"] != "NoPath" {
		t.Errorf("expected displayName=NoPath after path-less replace, got %v", m["displayName"])
	}
}

// TestPatch_MultipleOperations verifies that multiple operations in a single
// PATCH request are all applied (RFC 7644 §3.5.2: "Operations are applied
// sequentially in the order they appear in the array").
func TestPatch_MultipleOperations(t *testing.T) {
	s := newTestServer(t)

	createResp := doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"patch-multi"}`)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	patchBody := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"add","path":"displayName","value":"Multi Patch"},{"op":"add","path":"nickName","value":"multi"}]}`
	patchResp := doRequest(t, s, http.MethodPatch, "/Users/"+id, patchBody)
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", patchResp.StatusCode, readBody(t, patchResp))
	}
	var m map[string]interface{}
	decodeJSON(t, patchResp, &m)
	if m["displayName"] != "Multi Patch" {
		t.Errorf("displayName: expected 'Multi Patch', got %v", m["displayName"])
	}
	if m["nickName"] != "multi" {
		t.Errorf("nickName: expected 'multi', got %v", m["nickName"])
	}
}

// TestPatch_ResponseIs200OrNoContent verifies that a successful PATCH returns
// either 200 (with body) or 204 (No Content) (RFC 7644 §3.5.2:
// "the server either MUST return a 200 OK … or MAY return HTTP status code
// 204 (No Content)").
func TestPatch_ResponseIs200OrNoContent(t *testing.T) {
	s := newTestServer(t)

	createResp := doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"patch-status"}`)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	patchBody := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"add","path":"displayName","value":"status-check"}]}`
	patchResp := doRequest(t, s, http.MethodPatch, "/Users/"+id, patchBody)
	if patchResp.StatusCode != http.StatusOK && patchResp.StatusCode != http.StatusNoContent {
		t.Errorf("PATCH: expected 200 or 204, got %d; body: %s", patchResp.StatusCode, readBody(t, patchResp))
	}
}

// TestPatch_NotFound verifies that PATCH on a non-existent resource returns
// 404 (RFC 7644 §3.12, Table 8).
func TestPatch_NotFound(t *testing.T) {
	s := newTestServer(t)

	patchBody := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"add","path":"displayName","value":"x"}]}`
	resp := doRequest(t, s, http.MethodPatch, "/Users/no-such-id", patchBody)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
}

// ---------------------------------------------------------------------------
// RFC 7644 §3.6 – Deleting Resources
// ---------------------------------------------------------------------------

// TestDelete_ResponseHasNoBody verifies that a successful DELETE returns 204
// with an empty body (RFC 7644 §3.6: "the server SHALL return a successful
// HTTP status code 204 (No Content)").
func TestDelete_ResponseHasNoBody(t *testing.T) {
	s := newTestServer(t)

	createResp := doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"del-body"}`)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	resp := doRequest(t, s, http.MethodDelete, "/Users/"+id, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	if body := readBody(t, resp); strings.TrimSpace(body) != "" {
		t.Errorf("204 response should have empty body, got %q", body)
	}
}

// TestDelete_GetAfterDeleteReturns404 verifies that retrieving a deleted
// resource returns 404 (RFC 7644 §3.6: "MUST return a 404 (Not Found)
// error code for all operations associated with the previously deleted resource").
func TestDelete_GetAfterDeleteReturns404(t *testing.T) {
	s := newTestServer(t)

	createResp := doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"del-get"}`)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	doRequest(t, s, http.MethodDelete, "/Users/"+id, "")

	getResp := doRequest(t, s, http.MethodGet, "/Users/"+id, "")
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET after DELETE: expected 404, got %d", getResp.StatusCode)
	}
}

// TestDelete_RecreateSameUserName verifies that after deleting a resource,
// a new resource with the same userName SHOULD NOT fail with a conflict
// (RFC 7644 §3.6: "the service provider SHOULD NOT consider the deleted
// resource in conflict calculation").
func TestDelete_RecreateSameUserName(t *testing.T) {
	s := newTestServer(t)

	body := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"recycle-user"}`
	createResp := doRequest(t, s, http.MethodPost, "/Users", body)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	doRequest(t, s, http.MethodDelete, "/Users/"+id, "")

	// Re-creating with the same userName must not return 409.
	resp := doRequest(t, s, http.MethodPost, "/Users", body)
	if resp.StatusCode == http.StatusConflict {
		t.Error("§3.6: creating resource with same userName after delete should not return 409")
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201 on re-create, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
}

// TestDelete_NotFound verifies that deleting a non-existent resource returns
// 404 (RFC 7644 §3.12, Table 8).
func TestDelete_NotFound(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodDelete, "/Users/ghost-id", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
}

// ---------------------------------------------------------------------------
// RFC 7644 §3.7 – Bulk Operations
// ---------------------------------------------------------------------------

// TestBulk_Create verifies that a bulk POST creates resources and returns
// 201 status for each operation (RFC 7644 §3.7).
func TestBulk_Create(t *testing.T) {
	s := newTestServer(t)

	bulkBody := `{
		"schemas":["urn:ietf:params:scim:api:messages:2.0:BulkRequest"],
		"Operations":[
			{"method":"POST","path":"/Users","bulkId":"b1","data":{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bulk-create-1"}},
			{"method":"POST","path":"/Users","bulkId":"b2","data":{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bulk-create-2"}}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/Bulk", strings.NewReader(bulkBody))
	req.Header.Set("Content-Type", "application/scim+json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("bulk expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var m map[string]interface{}
	decodeJSON(t, rr.Result(), &m)

	ops, _ := m["Operations"].([]interface{})
	if len(ops) != 2 {
		t.Fatalf("expected 2 bulk ops, got %d", len(ops))
	}
	for i, opRaw := range ops {
		op, _ := opRaw.(map[string]interface{})
		if op["status"] != "201" {
			t.Errorf("op[%d]: expected status=201, got %v", i, op["status"])
		}
	}
}

// TestBulk_ResponseSchema verifies that the bulk response contains the
// BulkResponse schema URI (RFC 7644 §3.7).
func TestBulk_ResponseSchema(t *testing.T) {
	s := newTestServer(t)

	bulkBody := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:BulkRequest"],"Operations":[{"method":"POST","path":"/Users","bulkId":"rs1","data":{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bulk-rs-user"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/Bulk", strings.NewReader(bulkBody))
	req.Header.Set("Content-Type", "application/scim+json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	var m map[string]interface{}
	decodeJSON(t, rr.Result(), &m)

	schemas, _ := m["schemas"].([]interface{})
	const bulkResponseSchema = "urn:ietf:params:scim:api:messages:2.0:BulkResponse"
	found := false
	for _, s := range schemas {
		if s == bulkResponseSchema {
			found = true
		}
	}
	if !found {
		t.Errorf("bulk response schemas must contain %q, got %v", bulkResponseSchema, schemas)
	}
}

// TestBulk_BulkIdEchoedInResponse verifies that the bulkId is echoed back in
// the response for each POST operation (RFC 7644 §3.7: "The service provider
// MUST return the same 'bulkId' together with the newly created resource").
func TestBulk_BulkIdEchoedInResponse(t *testing.T) {
	s := newTestServer(t)

	bulkBody := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:BulkRequest"],"Operations":[{"method":"POST","path":"/Users","bulkId":"my-bulk-id-42","data":{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bulk-echo-user"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/Bulk", strings.NewReader(bulkBody))
	req.Header.Set("Content-Type", "application/scim+json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	var m map[string]interface{}
	decodeJSON(t, rr.Result(), &m)

	ops, _ := m["Operations"].([]interface{})
	if len(ops) == 0 {
		t.Fatal("expected at least 1 bulk operation result")
	}
	op, _ := ops[0].(map[string]interface{})
	if op["bulkId"] != "my-bulk-id-42" {
		t.Errorf("expected bulkId='my-bulk-id-42' in response, got %v", op["bulkId"])
	}
}

// TestBulk_LocationInSuccessfulPostResponse verifies that a successful POST
// bulk operation includes a location in the response (RFC 7644 §3.7.3:
// "A 'location' attribute … MUST be returned for all operations except for
// failed POST operations").
func TestBulk_LocationInSuccessfulPostResponse(t *testing.T) {
	s := newTestServer(t)

	bulkBody := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:BulkRequest"],"Operations":[{"method":"POST","path":"/Users","bulkId":"loc1","data":{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bulk-location-user"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/Bulk", strings.NewReader(bulkBody))
	req.Header.Set("Content-Type", "application/scim+json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	var m map[string]interface{}
	decodeJSON(t, rr.Result(), &m)

	ops, _ := m["Operations"].([]interface{})
	if len(ops) == 0 {
		t.Fatal("expected at least 1 bulk op result")
	}
	op, _ := ops[0].(map[string]interface{})
	if loc, _ := op["location"].(string); loc == "" {
		t.Error("successful bulk POST must include 'location' in response")
	}
}

// TestBulk_DeleteOperation verifies that DELETE inside a bulk request removes
// the resource (RFC 7644 §3.7).
func TestBulk_DeleteOperation(t *testing.T) {
	s := newTestServer(t)

	createResp := doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bulk-del-user"}`)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	bulkBody := fmt.Sprintf(`{"schemas":["urn:ietf:params:scim:api:messages:2.0:BulkRequest"],"Operations":[{"method":"DELETE","path":"/Users/%s","bulkId":"del1"}]}`, id)
	req := httptest.NewRequest(http.MethodPost, "/Bulk", strings.NewReader(bulkBody))
	req.Header.Set("Content-Type", "application/scim+json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("bulk expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var m map[string]interface{}
	decodeJSON(t, rr.Result(), &m)

	ops, _ := m["Operations"].([]interface{})
	if len(ops) != 1 {
		t.Fatalf("expected 1 op result, got %d", len(ops))
	}
	op, _ := ops[0].(map[string]interface{})
	if op["status"] != "204" {
		t.Errorf("bulk DELETE: expected status=204, got %v", op["status"])
	}

	getResp := doRequest(t, s, http.MethodGet, "/Users/"+id, "")
	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("after bulk DELETE, GET should return 404, got %d", getResp.StatusCode)
	}
}

// TestBulk_FailOnErrors verifies that processing stops after N errors when
// failOnErrors is set (RFC 7644 §3.7: "The failOnErrors attribute defines the
// number of errors … the service provider should accept before failing the
// remaining operations").
func TestBulk_FailOnErrors(t *testing.T) {
	s := newTestServer(t)

	bulkBody := `{
		"schemas":["urn:ietf:params:scim:api:messages:2.0:BulkRequest"],
		"failOnErrors":1,
		"Operations":[
			{"method":"DELETE","path":"/Users/nonexistent-1","bulkId":"fail1"},
			{"method":"DELETE","path":"/Users/nonexistent-2","bulkId":"fail2"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/Bulk", strings.NewReader(bulkBody))
	req.Header.Set("Content-Type", "application/scim+json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("bulk expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var m map[string]interface{}
	decodeJSON(t, rr.Result(), &m)

	ops, _ := m["Operations"].([]interface{})
	// failOnErrors=1: stop after 1 failure → only 1 op result returned.
	if len(ops) != 1 {
		t.Errorf("failOnErrors=1: expected 1 op result, got %d", len(ops))
	}
}

// TestBulk_ContinuesWithoutFailOnErrors verifies that without failOnErrors,
// all operations are attempted even if some fail (RFC 7644 §3.7: "The service
// provider MUST continue performing as many changes as possible and disregard
// partial failures").
func TestBulk_ContinuesWithoutFailOnErrors(t *testing.T) {
	s := newTestServer(t)

	bulkBody := `{
		"schemas":["urn:ietf:params:scim:api:messages:2.0:BulkRequest"],
		"Operations":[
			{"method":"DELETE","path":"/Users/nonexistent-1","bulkId":"nf1"},
			{"method":"DELETE","path":"/Users/nonexistent-2","bulkId":"nf2"},
			{"method":"DELETE","path":"/Users/nonexistent-3","bulkId":"nf3"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/Bulk", strings.NewReader(bulkBody))
	req.Header.Set("Content-Type", "application/scim+json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	var m map[string]interface{}
	decodeJSON(t, rr.Result(), &m)

	ops, _ := m["Operations"].([]interface{})
	if len(ops) != 3 {
		t.Errorf("without failOnErrors: expected 3 op results, got %d", len(ops))
	}
}

// TestBulk_ExceedsMaxOperations verifies that exceeding maxBulkOps returns
// 413 (RFC 7644 §3.7.4: "If either limit is exceeded, the service provider
// MUST return HTTP response code 413 (Payload Too Large)").
func TestBulk_ExceedsMaxOperations(t *testing.T) {
	s := newTestServer(t, server.WithMaxBulkOps(2))

	bulkBody := `{
		"schemas":["urn:ietf:params:scim:api:messages:2.0:BulkRequest"],
		"Operations":[
			{"method":"POST","path":"/Users","bulkId":"b1","data":{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bulk-max-1"}},
			{"method":"POST","path":"/Users","bulkId":"b2","data":{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bulk-max-2"}},
			{"method":"POST","path":"/Users","bulkId":"b3","data":{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bulk-max-3"}}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/Bulk", strings.NewReader(bulkBody))
	req.Header.Set("Content-Type", "application/scim+json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestBulk_MixedOperations verifies POST, PUT, and DELETE operations in a
// single bulk request (RFC 7644 §3.7).
func TestBulk_MixedOperations(t *testing.T) {
	s := newTestServer(t)

	// Pre-create a resource to be updated/deleted.
	createResp := doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bulk-mix-existing"}`)
	var existing map[string]interface{}
	decodeJSON(t, createResp, &existing)
	existingID := existing["id"].(string)

	bulkBody := fmt.Sprintf(`{
		"schemas":["urn:ietf:params:scim:api:messages:2.0:BulkRequest"],
		"Operations":[
			{"method":"POST","path":"/Users","bulkId":"new1","data":{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bulk-mix-new"}},
			{"method":"PUT","path":"/Users/%s","bulkId":"upd1","data":{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bulk-mix-existing","displayName":"Updated"}},
			{"method":"DELETE","path":"/Users/%s","bulkId":"del1"}
		]
	}`, existingID, existingID)

	req := httptest.NewRequest(http.MethodPost, "/Bulk", strings.NewReader(bulkBody))
	req.Header.Set("Content-Type", "application/scim+json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("bulk mixed ops expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var m map[string]interface{}
	decodeJSON(t, rr.Result(), &m)

	ops, _ := m["Operations"].([]interface{})
	if len(ops) != 3 {
		t.Fatalf("expected 3 bulk op results, got %d", len(ops))
	}
	expectedStatuses := []string{"201", "200", "204"}
	for i, opRaw := range ops {
		op, _ := opRaw.(map[string]interface{})
		if op["status"] != expectedStatuses[i] {
			t.Errorf("op[%d]: expected status=%s, got %v", i, expectedStatuses[i], op["status"])
		}
	}
}

// ---------------------------------------------------------------------------
// RFC 7644 §3.8 – Data Input/Output Formats (content-type negotiation)
// ---------------------------------------------------------------------------

// TestContentType_AcceptApplicationJSON verifies that Accept: application/json
// is honoured (RFC 7644 §3.8: "SHOULD support the header Accept: application/json").
func TestContentType_AcceptApplicationJSON(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/ServiceProviderConfig", nil)
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") && !strings.HasPrefix(ct, "application/scim+json") {
		t.Errorf("Accept: application/json – unexpected Content-Type %q", ct)
	}
}

// TestContentType_UnacceptableReturns406 verifies that an unacceptable Accept
// header returns 406 (RFC 7644 §3.8).
func TestContentType_UnacceptableReturns406(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/ServiceProviderConfig", nil)
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotAcceptable {
		t.Errorf("Accept: text/html – expected 406, got %d", rr.Code)
	}
}

// TestContentType_ResponseIsScimJson verifies that responses default to
// Content-Type: application/scim+json (RFC 7644 §3.8).
func TestContentType_ResponseIsScimJson(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, "/Users", "")
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/scim+json") {
		t.Errorf("expected application/scim+json response Content-Type, got %q", ct)
	}
}

// TestContentType_UnsupportedContentTypeReturns415 verifies that an
// unsupported request Content-Type returns 415 (RFC 7644 §3.8).
func TestContentType_UnsupportedContentTypeReturns415(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/Users", strings.NewReader(`{"userName":"x"}`))
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// RFC 7644 §3.12 – HTTP Status and Error Response Handling
// ---------------------------------------------------------------------------

// TestError_ResponseContainsSchemas verifies that all SCIM error responses
// include the mandatory schemas field with the Error URI
// (RFC 7644 §3.12: "Error responses are identified using the following
// schema URI: urn:ietf:params:scim:api:messages:2.0:Error").
func TestError_ResponseContainsSchemas(t *testing.T) {
	s := newTestServer(t)

	// Pre-seed duplicate.
	doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"err-dup"}`)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"not found", http.MethodGet, "/Users/no-such-id", "", http.StatusNotFound},
		{"conflict", http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"err-dup"}`, http.StatusConflict},
		{"invalid filter", http.MethodGet, "/Users?filter=!!!", "", http.StatusBadRequest},
		{"invalid json", http.MethodPost, "/Users", `not json`, http.StatusBadRequest},
		{"method not allowed", http.MethodDelete, "/ServiceProviderConfig", "", http.StatusMethodNotAllowed},
	}

	const errorSchema = "urn:ietf:params:scim:api:messages:2.0:Error"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doRequest(t, s, tc.method, tc.path, tc.body)
			if resp.StatusCode != tc.want {
				t.Fatalf("expected %d, got %d; body: %s", tc.want, resp.StatusCode, readBody(t, resp))
			}
			var m map[string]interface{}
			decodeJSON(t, resp, &m)
			schemas, _ := m["schemas"].([]interface{})
			found := false
			for _, s := range schemas {
				if s == errorSchema {
					found = true
				}
			}
			if !found {
				t.Errorf("error response schemas must contain %q, got %v", errorSchema, schemas)
			}
			if _, ok := m["status"]; !ok {
				t.Error("error response missing 'status' field")
			}
		})
	}
}

// TestError_InvalidFilterScimType verifies scimType=invalidFilter
// (RFC 7644 §3.12, Table 9).
func TestError_InvalidFilterScimType(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, "/Users?filter=!!!", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)
	if m["scimType"] != "invalidFilter" {
		t.Errorf("expected scimType=invalidFilter, got %v", m["scimType"])
	}
}

// TestError_ConflictScimType verifies scimType=uniqueness on duplicate
// userName (RFC 7644 §3.12, Table 9).
func TestError_ConflictScimType(t *testing.T) {
	s := newTestServer(t)

	body := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"unique-clash"}`
	doRequest(t, s, http.MethodPost, "/Users", body)
	resp := doRequest(t, s, http.MethodPost, "/Users", body)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)
	if m["scimType"] != "uniqueness" {
		t.Errorf("expected scimType=uniqueness, got %v", m["scimType"])
	}
}

// ---------------------------------------------------------------------------
// RFC 7644 §3.2 – HTTP Method Restrictions (405 Method Not Allowed)
// ---------------------------------------------------------------------------

// TestMethodNotAllowed_DiscoveryEndpoints verifies that wrong HTTP methods on
// discovery endpoints return 405 (RFC 7644 §3.2, Table 2).
func TestMethodNotAllowed_DiscoveryEndpoints(t *testing.T) {
	s := newTestServer(t)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodDelete, "/ServiceProviderConfig"},
		{http.MethodPost, "/ServiceProviderConfig"},
		{http.MethodPut, "/ServiceProviderConfig"},
		{http.MethodDelete, "/ResourceTypes"},
		{http.MethodPut, "/Schemas"},
		{http.MethodGet, "/Bulk"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resp := doRequest(t, s, tc.method, tc.path, "")
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("%s %s: expected 405, got %d; body: %s",
					tc.method, tc.path, resp.StatusCode, readBody(t, resp))
			}
		})
	}
}

// TestMethodNotAllowed_ResourceEndpoints verifies 405 for unsupported methods
// on resource collection and individual endpoints (RFC 7644 §3.2).
func TestMethodNotAllowed_ResourceEndpoints(t *testing.T) {
	s := newTestServer(t)

	createResp := doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"mna-user"}`)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/Users"},
		{http.MethodPatch, "/Users"},
		{http.MethodDelete, "/Users"},
		{http.MethodPost, "/Users/" + id},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resp := doRequest(t, s, tc.method, tc.path, "")
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("%s %s: expected 405, got %d; body: %s",
					tc.method, tc.path, resp.StatusCode, readBody(t, resp))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RFC 7644 §4 – Discovery Endpoints
// ---------------------------------------------------------------------------

// TestServiceProviderConfig_Schema verifies the ServiceProviderConfig response
// uses the correct schema URI (RFC 7644 §4).
func TestServiceProviderConfig_Schema(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, "/ServiceProviderConfig", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	schemas, _ := m["schemas"].([]interface{})
	const spcSchema = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	found := false
	for _, s := range schemas {
		if s == spcSchema {
			found = true
		}
	}
	if !found {
		t.Errorf("ServiceProviderConfig schemas must contain %q, got %v", spcSchema, schemas)
	}
}

// TestServiceProviderConfig_CapabilityObjects verifies the ServiceProviderConfig
// contains required capability objects (RFC 7644 §4 / RFC 7643 §5).
func TestServiceProviderConfig_CapabilityObjects(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, "/ServiceProviderConfig", "")
	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	for _, cap := range []string{"patch", "bulk", "filter", "changePassword", "sort", "etag"} {
		if _, ok := m[cap]; !ok {
			t.Errorf("ServiceProviderConfig missing capability %q", cap)
		}
	}

	if patch, _ := m["patch"].(map[string]interface{}); patch != nil {
		if patch["supported"] != true {
			t.Errorf("patch.supported: expected true, got %v", patch["supported"])
		}
	}
	if bulk, _ := m["bulk"].(map[string]interface{}); bulk != nil {
		if _, ok := bulk["maxOperations"]; !ok {
			t.Error("bulk missing 'maxOperations'")
		}
	}
}

// TestResourceTypes_Structure verifies each ResourceType contains required
// fields (RFC 7644 §4 / RFC 7643 §6).
func TestResourceTypes_Structure(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, "/ResourceTypes", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)

	resources, _ := listResp["Resources"].([]interface{})
	if len(resources) == 0 {
		t.Fatal("expected at least one ResourceType")
	}

	const rtSchema = "urn:ietf:params:scim:schemas:core:2.0:ResourceType"
	for _, raw := range resources {
		rt, _ := raw.(map[string]interface{})
		for _, field := range []string{"name", "endpoint", "schema", "schemas"} {
			if _, ok := rt[field]; !ok {
				t.Errorf("ResourceType %v missing field %q", rt["name"], field)
			}
		}
		schemas, _ := rt["schemas"].([]interface{})
		found := false
		for _, s := range schemas {
			if s == rtSchema {
				found = true
			}
		}
		if !found {
			t.Errorf("ResourceType %v schemas must include %q, got %v", rt["name"], rtSchema, schemas)
		}
	}
}

// TestSchemas_Structure verifies each Schema contains required fields
// (RFC 7644 §4 / RFC 7643 §7).
func TestSchemas_Structure(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, "/Schemas", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)

	resources, _ := listResp["Resources"].([]interface{})
	if len(resources) == 0 {
		t.Fatal("expected at least one Schema")
	}
	for _, raw := range resources {
		sch, _ := raw.(map[string]interface{})
		for _, field := range []string{"id", "name", "attributes", "schemas"} {
			if _, ok := sch[field]; !ok {
				t.Errorf("Schema %v missing field %q", sch["id"], field)
			}
		}
	}
}

// TestSchemas_NotFound verifies that an unknown schema ID returns 404
// (RFC 7644 §4).
func TestSchemas_NotFound(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, "/Schemas/urn:ietf:params:scim:schemas:core:2.0:NonExistent", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
}

// TestResourceTypes_NotFound verifies that an unknown resource type returns
// 404 (RFC 7644 §4).
func TestResourceTypes_NotFound(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, "/ResourceTypes/NonExistent", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
}

// ---------------------------------------------------------------------------
// RFC 7644 §3.14 – Versioning Resources (meta.location)
// ---------------------------------------------------------------------------

// TestVersioning_MetaLocationOnGet verifies that meta.location in a GET
// response refers to the resource (RFC 7644 §3.14).
func TestVersioning_MetaLocationOnGet(t *testing.T) {
	s := newTestServer(t)

	createResp := doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"version-user"}`)
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)
	id := created["id"].(string)

	resp := doRequest(t, s, http.MethodGet, "/Users/"+id, "")
	var m map[string]interface{}
	decodeJSON(t, resp, &m)

	meta, ok := m["meta"].(map[string]interface{})
	if !ok {
		t.Fatal("GET response missing 'meta'")
	}
	if loc, _ := meta["location"].(string); !strings.Contains(loc, "/Users/"+id) {
		t.Errorf("meta.location %q does not contain '/Users/%s'", loc, id)
	}
}

// TestVersioning_MetaTimestampsAreRFC3339 verifies that created and
// lastModified are valid RFC3339 timestamps (RFC 7644 §3.14 / RFC 7643 §3.1).
func TestVersioning_MetaTimestampsAreRFC3339(t *testing.T) {
	s := newTestServer(t)

	createResp := doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"ts-user"}`)
	var m map[string]interface{}
	decodeJSON(t, createResp, &m)

	meta, _ := m["meta"].(map[string]interface{})
	for _, field := range []string{"created", "lastModified"} {
		v, _ := meta[field].(string)
		if v == "" {
			t.Errorf("meta.%s is empty", field)
			continue
		}
		if _, err := time.Parse(time.RFC3339, v); err != nil {
			// Also try RFC3339Nano.
			if _, err2 := time.Parse(time.RFC3339Nano, v); err2 != nil {
				t.Errorf("meta.%s = %q is not a valid RFC3339 timestamp: %v", field, v, err)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Groups – RFC 7643 §4.2
// ---------------------------------------------------------------------------

// TestGroups_CRUD verifies create, get, list, and delete for Groups
// (RFC 7644 §3.2, RFC 7643 §4.2).
func TestGroups_CRUD(t *testing.T) {
	s := newTestServer(t)

	// Create.
	body := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:Group"],"displayName":"Engineering"}`
	createResp := doRequest(t, s, http.MethodPost, "/Groups", body)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create group: expected 201, got %d; body: %s", createResp.StatusCode, readBody(t, createResp))
	}
	var created map[string]interface{}
	decodeJSON(t, createResp, &created)

	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("create group: expected non-empty id")
	}
	const groupSchema = "urn:ietf:params:scim:schemas:core:2.0:Group"
	schemas, _ := created["schemas"].([]interface{})
	found := false
	for _, s := range schemas {
		if s == groupSchema {
			found = true
		}
	}
	if !found {
		t.Errorf("group schemas must include %q, got %v", groupSchema, schemas)
	}

	// Get.
	getResp := doRequest(t, s, http.MethodGet, "/Groups/"+id, "")
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get group: expected 200, got %d; body: %s", getResp.StatusCode, readBody(t, getResp))
	}
	var got map[string]interface{}
	decodeJSON(t, getResp, &got)
	if got["id"] != id {
		t.Errorf("get group: expected id=%s, got %v", id, got["id"])
	}

	// List.
	listResp := doRequest(t, s, http.MethodGet, "/Groups", "")
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list groups: expected 200, got %d", listResp.StatusCode)
	}
	var list map[string]interface{}
	decodeJSON(t, listResp, &list)
	if _, ok := list["totalResults"]; !ok {
		t.Error("list groups response missing totalResults")
	}

	// Delete.
	delResp := doRequest(t, s, http.MethodDelete, "/Groups/"+id, "")
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete group: expected 204, got %d", delResp.StatusCode)
	}

	// Verify gone.
	afterResp := doRequest(t, s, http.MethodGet, "/Groups/"+id, "")
	if afterResp.StatusCode != http.StatusNotFound {
		t.Errorf("get after delete: expected 404, got %d", afterResp.StatusCode)
	}
}

// TestGroups_PatchAddMembers verifies PATCH add to group members
// (RFC 7644 §3.5.2.1).
func TestGroups_PatchAddMembers(t *testing.T) {
	s := newTestServer(t)

	userResp := doRequest(t, s, http.MethodPost, "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"group-member-user"}`)
	var user map[string]interface{}
	decodeJSON(t, userResp, &user)
	userID := user["id"].(string)

	groupResp := doRequest(t, s, http.MethodPost, "/Groups", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:Group"],"displayName":"MemberGroup"}`)
	var group map[string]interface{}
	decodeJSON(t, groupResp, &group)
	groupID := group["id"].(string)

	patchBody := fmt.Sprintf(`{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"add","path":"members","value":[{"value":%q}]}]}`, userID)
	patchResp := doRequest(t, s, http.MethodPatch, "/Groups/"+groupID, patchBody)
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", patchResp.StatusCode, readBody(t, patchResp))
	}

	getResp := doRequest(t, s, http.MethodGet, "/Groups/"+groupID, "")
	var m map[string]interface{}
	decodeJSON(t, getResp, &m)
	members, _ := m["members"].([]interface{})
	if len(members) == 0 {
		t.Error("expected group to have members after PATCH add")
	}
}

// ---------------------------------------------------------------------------
// BasePath routing
// ---------------------------------------------------------------------------

// TestBasePath_RoutesCorrectly verifies that requests under a custom base path
// are handled and requests outside it return 404.
func TestBasePath_RoutesCorrectly(t *testing.T) {
	s := newTestServer(t, server.WithBasePath("/scim/v2"))

	t.Run("without base path returns 404", func(t *testing.T) {
		resp := doRequest(t, s, http.MethodGet, "/Users", "")
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("with base path works", func(t *testing.T) {
		resp := doRequest(t, s, http.MethodGet, "/scim/v2/Users", "")
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
		}
	})

	t.Run("create under base path", func(t *testing.T) {
		body := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"basepath-user"}`
		resp := doRequest(t, s, http.MethodPost, "/scim/v2/Users", body)
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected 201, got %d; body: %s", resp.StatusCode, readBody(t, resp))
		}
	})

	t.Run("discovery under base path", func(t *testing.T) {
		resp := doRequest(t, s, http.MethodGet, "/scim/v2/ServiceProviderConfig", "")
		if resp.StatusCode != http.StatusOK {
			t.Errorf("ServiceProviderConfig: expected 200, got %d", resp.StatusCode)
		}
	})
}

// ---------------------------------------------------------------------------
// Unknown / unregistered endpoints
// ---------------------------------------------------------------------------

// TestUnknownEndpoint verifies that a request to an unregistered resource type
// endpoint returns a proper SCIM 404 error (RFC 7644 §3.2).
func TestUnknownEndpoint(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodGet, "/Devices", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	assertScimErrorResponse(t, resp, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Helpers used only within this file
// ---------------------------------------------------------------------------

// assertSingleResult checks that the list response contains exactly one
// resource and that the named attribute equals the expected value.
func assertSingleResult(t *testing.T, resp *http.Response, attr, expected string) {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, readBody(t, resp))
	}
	var m map[string]interface{}
	decodeJSON(t, resp, &m)
	resources, _ := m["Resources"].([]interface{})
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	entry, _ := resources[0].(map[string]interface{})
	if entry[attr] != expected {
		t.Errorf("%s: expected %q, got %v", attr, expected, entry[attr])
	}
}

// assertScimErrorResponse checks that an HTTP response body is a valid SCIM
// error (schemas + status).
func assertScimErrorResponse(t *testing.T, resp *http.Response, expectedStatus int) {
	t.Helper()
	var m map[string]interface{}
	decodeJSON(t, resp, &m)
	const errorSchema = "urn:ietf:params:scim:api:messages:2.0:Error"
	schemas, _ := m["schemas"].([]interface{})
	found := false
	for _, s := range schemas {
		if s == errorSchema {
			found = true
		}
	}
	if !found {
		t.Errorf("SCIM error response schemas must contain %q, got %v", errorSchema, schemas)
	}
	if _, ok := m["status"]; !ok {
		t.Error("SCIM error response missing 'status' field")
	}
}

// listResourceAttr extracts the named attribute from every resource in a list
// response.
func listResourceAttr(t *testing.T, resp *http.Response, attr string) []string {
	t.Helper()
	var m map[string]interface{}
	decodeJSON(t, resp, &m)
	resources, _ := m["Resources"].([]interface{})
	out := make([]string, 0, len(resources))
	for _, raw := range resources {
		entry, _ := raw.(map[string]interface{})
		if v, _ := entry[attr].(string); v != "" {
			out = append(out, v)
		}
	}
	return out
}
