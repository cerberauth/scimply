package compliance

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const (
	userSchemaURI = "urn:ietf:params:scim:schemas:core:2.0:User"

	opAdd         = "add"
	pathAttr      = "path"
	nickNameAttr  = "nickName"
	userNameAttr  = "userName"
	emailsAttr    = "emails"
	resourcesAttr = "Resources"
)

func testCreateUser(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	id, resource := createUser(t, cfg, uniqueSuffix(t, ""))
	if id == "" {
		return
	}

	checkSchemas(t, resource, userSchemaURI)

	if un, _ := resource[userNameAttr].(string); !strings.HasPrefix(un, "test-user-") {
		t.Errorf("userName %q does not start with 'test-user-'", un)
	}

	if active, _ := resource["active"].(bool); !active {
		t.Errorf("expected active=true, got %v", resource["active"])
	}
}

func testGetUser(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	id, _ := createUser(t, cfg, uniqueSuffix(t, ""))
	if id == "" {
		return
	}

	resp := doRequest(t, cfg, http.MethodGet, fmt.Sprintf("/Users/%s", id), nil)
	if resp.StatusCode != http.StatusOK {
		drainAndClose(resp)
		t.Fatalf("GET /Users/%s: expected 200, got %d", id, resp.StatusCode)
	}

	var body map[string]interface{}
	mustDecodeJSON(t, resp, &body)

	checkSchemas(t, body, userSchemaURI)
	gotID := checkID(t, body)
	if gotID != id {
		t.Errorf("GET returned id %q, expected %q", gotID, id)
	}
}

func testListUsers(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	createUser(t, cfg, uniqueSuffix(t, ""))

	resp := doRequest(t, cfg, http.MethodGet, "/Users", nil)
	if resp.StatusCode != http.StatusOK {
		drainAndClose(resp)
		t.Fatalf("GET /Users: expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	mustDecodeJSON(t, resp, &body)

	checkSchemas(t, body, listResponseSchema)

	if _, ok := body["totalResults"]; !ok {
		t.Errorf("ListResponse missing 'totalResults'")
	}
	if _, ok := body["startIndex"]; !ok {
		t.Errorf("ListResponse missing 'startIndex'")
	}
	if _, ok := body["itemsPerPage"]; !ok {
		t.Errorf("ListResponse missing 'itemsPerPage'")
	}
	if _, ok := body[resourcesAttr]; !ok {
		t.Errorf("ListResponse missing 'Resources'")
	}
}

func testFilterUsers(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	suffix := uniqueSuffix(t, "")
	id, resource := createUser(t, cfg, suffix)
	if id == "" {
		return
	}

	userName, _ := resource[userNameAttr].(string)
	filter := fmt.Sprintf("userName eq %q", userName)
	path := "/Users?filter=" + encodeQueryParam(filter)

	resp := doRequest(t, cfg, http.MethodGet, path, nil)
	if resp.StatusCode != http.StatusOK {
		drainAndClose(resp)
		t.Fatalf("GET /Users?filter=...: expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	mustDecodeJSON(t, resp, &body)

	checkSchemas(t, body, listResponseSchema)

	resources, _ := body[resourcesAttr].([]interface{})
	if len(resources) == 0 {
		t.Errorf("filter returned 0 results for userName eq %q", userName)
		return
	}

	for i, raw := range resources {
		entry, _ := raw.(map[string]interface{})
		got, _ := entry[userNameAttr].(string)
		if got != userName {
			t.Errorf("Resources[%d]: userName=%q, expected %q", i, got, userName)
		}
	}
}

func testPagination(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	createUser(t, cfg, uniqueSuffix(t, "p1"))
	createUser(t, cfg, uniqueSuffix(t, "p2"))

	resp := doRequest(t, cfg, http.MethodGet, "/Users?startIndex=1&count=1", nil)
	if resp.StatusCode != http.StatusOK {
		drainAndClose(resp)
		t.Fatalf("GET /Users?startIndex=1&count=1: expected 200, got %d", resp.StatusCode)
	}
	var page1 map[string]interface{}
	mustDecodeJSON(t, resp, &page1)

	resources1, _ := page1[resourcesAttr].([]interface{})
	if len(resources1) > 1 {
		t.Errorf("count=1 but received %d resources", len(resources1))
	}

	total, _ := page1["totalResults"].(float64)
	if total < 2 {
		t.Logf("totalResults=%v, expected ≥ 2 (may have other users from prior tests)", total)
	}

	resp2 := doRequest(t, cfg, http.MethodGet, "/Users?startIndex=2&count=1", nil)
	if resp2.StatusCode != http.StatusOK {
		drainAndClose(resp2)
		t.Fatalf("GET /Users?startIndex=2&count=1: expected 200, got %d", resp2.StatusCode)
	}
	var page2 map[string]interface{}
	mustDecodeJSON(t, resp2, &page2)

	startIndex2, _ := page2["startIndex"].(float64)
	if startIndex2 != 2 {
		t.Errorf("startIndex expected 2, got %v", startIndex2)
	}
}

func testReplaceUser(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	id, resource := createUser(t, cfg, uniqueSuffix(t, ""))
	if id == "" {
		return
	}

	resource["displayName"] = "Replaced Name"
	resource[schemasAttr] = []interface{}{userSchemaURI}

	resp := doRequest(t, cfg, http.MethodPut, fmt.Sprintf("/Users/%s", id), resource)
	if resp.StatusCode != http.StatusOK {
		data, _ := readBody(resp)
		t.Fatalf("PUT /Users/%s: expected 200, got %d\nbody: %s", id, resp.StatusCode, data)
	}

	var body map[string]interface{}
	mustDecodeJSON(t, resp, &body)

	checkSchemas(t, body, userSchemaURI)
	if dn, _ := body["displayName"].(string); dn != "Replaced Name" {
		t.Errorf("PUT: displayName=%q, expected %q", dn, "Replaced Name")
	}
}

func testPatchAdd(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	id, _ := createUser(t, cfg, uniqueSuffix(t, ""))
	if id == "" {
		return
	}

	patch := buildPatch([]map[string]interface{}{
		{
			"op":      opAdd,
			pathAttr:  nickNameAttr,
			valueAttr: "patches-nickname",
		},
	})

	resp := doRequest(t, cfg, http.MethodPatch, fmt.Sprintf("/Users/%s", id), patch)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		data, _ := readBody(resp)
		t.Fatalf("PATCH add /Users/%s: expected 200 or 204, got %d\nbody: %s", id, resp.StatusCode, data)
	}
	drainAndClose(resp)

	get := doRequest(t, cfg, http.MethodGet, fmt.Sprintf("/Users/%s", id), nil)
	var body map[string]interface{}
	mustDecodeJSON(t, get, &body)

	if nn, _ := body[nickNameAttr].(string); nn != "patches-nickname" {
		t.Errorf("after PATCH add, nickName=%q, expected %q", nn, "patches-nickname")
	}
}

func testPatchRemove(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	id, _ := createUser(t, cfg, uniqueSuffix(t, ""))
	if id == "" {
		return
	}

	addPatch := buildPatch([]map[string]interface{}{
		{"op": opAdd, pathAttr: nickNameAttr, valueAttr: "to-be-removed"},
	})
	addResp := doRequest(t, cfg, http.MethodPatch, fmt.Sprintf("/Users/%s", id), addPatch)
	drainAndClose(addResp)

	removePatch := buildPatch([]map[string]interface{}{
		{"op": "remove", pathAttr: nickNameAttr},
	})

	resp := doRequest(t, cfg, http.MethodPatch, fmt.Sprintf("/Users/%s", id), removePatch)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		data, _ := readBody(resp)
		t.Fatalf("PATCH remove /Users/%s: expected 200 or 204, got %d\nbody: %s", id, resp.StatusCode, data)
	}
	drainAndClose(resp)

	get := doRequest(t, cfg, http.MethodGet, fmt.Sprintf("/Users/%s", id), nil)
	var body map[string]interface{}
	mustDecodeJSON(t, get, &body)

	if nn, exists := body[nickNameAttr]; exists && nn != nil && nn != "" {
		t.Errorf("after PATCH remove, nickName still present: %v", nn)
	}
}

func testPatchReplace(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	id, _ := createUser(t, cfg, uniqueSuffix(t, ""))
	if id == "" {
		return
	}

	patch := buildPatch([]map[string]interface{}{
		{"op": "replace", pathAttr: "displayName", valueAttr: "Patched Display"},
	})

	resp := doRequest(t, cfg, http.MethodPatch, fmt.Sprintf("/Users/%s", id), patch)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		data, _ := readBody(resp)
		t.Fatalf("PATCH replace /Users/%s: expected 200 or 204, got %d\nbody: %s", id, resp.StatusCode, data)
	}
	drainAndClose(resp)

	get := doRequest(t, cfg, http.MethodGet, fmt.Sprintf("/Users/%s", id), nil)
	var body map[string]interface{}
	mustDecodeJSON(t, get, &body)

	if dn, _ := body["displayName"].(string); dn != "Patched Display" {
		t.Errorf("after PATCH replace, displayName=%q, expected %q", dn, "Patched Display")
	}
}

func testPatchComplex(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	id, resource := createUser(t, cfg, uniqueSuffix(t, ""))
	if id == "" {
		return
	}

	emails, _ := resource[emailsAttr].([]interface{})
	if len(emails) == 0 {
		t.Skip("testPatchComplex: user has no emails, skipping complex patch test")
	}

	patch := buildPatch([]map[string]interface{}{
		{
			"op":      "replace",
			pathAttr:  "emails[type eq \"work\"].value",
			valueAttr: "updated@example.com",
		},
	})

	resp := doRequest(t, cfg, http.MethodPatch, fmt.Sprintf("/Users/%s", id), patch)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		data, _ := readBody(resp)
		t.Logf("PATCH complex /Users/%s: got %d (server may not support value filters)\nbody: %s", id, resp.StatusCode, data)

		return
	}
	drainAndClose(resp)

	get := doRequest(t, cfg, http.MethodGet, fmt.Sprintf("/Users/%s", id), nil)
	var body map[string]interface{}
	mustDecodeJSON(t, get, &body)

	updatedEmails, _ := body[emailsAttr].([]interface{})
	found := false
	for _, raw := range updatedEmails {
		entry, _ := raw.(map[string]interface{})
		if v, _ := entry[valueAttr].(string); v == "updated@example.com" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("after complex PATCH, expected email 'updated@example.com' not found in %v", updatedEmails)
	}
}

func testDeleteUser(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	resp := doRequest(t, cfg, http.MethodPost, "/Users", NewTestUser(uniqueSuffix(t, "")))
	if resp.StatusCode != http.StatusCreated {
		drainAndClose(resp)
		t.Skipf("testDeleteUser: POST /Users returned %d", resp.StatusCode)
		return
	}
	var created map[string]interface{}
	mustDecodeJSON(t, resp, &created)
	id := checkID(t, created)
	if id == "" {
		return
	}

	del := doRequest(t, cfg, http.MethodDelete, fmt.Sprintf("/Users/%s", id), nil)
	drainAndClose(del)
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /Users/%s: expected 204, got %d", id, del.StatusCode)
	}

	get := doRequest(t, cfg, http.MethodGet, fmt.Sprintf("/Users/%s", id), nil)
	if get.StatusCode != http.StatusNotFound {
		drainAndClose(get)
		t.Errorf("GET after DELETE /Users/%s: expected 404, got %d", id, get.StatusCode)
		return
	}
	checkErrorResponse(t, get, http.StatusNotFound)
}

func buildPatch(ops []map[string]interface{}) map[string]interface{} {
	iops := make([]interface{}, len(ops))
	for i, op := range ops {
		iops[i] = op
	}
	return map[string]interface{}{
		schemasAttr:  []interface{}{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": iops,
	}
}

func encodeQueryParam(s string) string {
	return strings.NewReplacer(
		" ", "%20",
		"\"", "%22",
		"(", "%28",
		")", "%29",
		"[", "%5B",
		"]", "%5D",
	).Replace(s)
}

func readBody(r *http.Response) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, nil
	}
	defer func() { _ = r.Body.Close() }()
	return readAll(r)
}

func readAll(r *http.Response) ([]byte, error) {
	buf := new(strings.Builder)
	tmp := make([]byte, 512)
	for {
		n, err := r.Body.Read(tmp)
		buf.Write(tmp[:n])
		if err != nil {
			break
		}
	}
	return []byte(buf.String()), nil
}
