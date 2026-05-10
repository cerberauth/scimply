package compliance

import (
	"fmt"
	"net/http"
	"testing"
)

const (
	groupSchemaURI = "urn:ietf:params:scim:schemas:core:2.0:Group"
	membersAttr    = "members"
)

func testCreateGroup(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	id, resource := createGroup(t, cfg, uniqueSuffix(t, ""))
	if id == "" {
		return
	}

	checkSchemas(t, resource, groupSchemaURI)

	if dn, _ := resource["displayName"].(string); dn == "" {
		t.Errorf("Group response missing 'displayName'")
	}
}

func testGroupMembership(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	userID, _ := createUser(t, cfg, uniqueSuffix(t, "u"))
	if userID == "" {
		return
	}
	groupID, _ := createGroup(t, cfg, uniqueSuffix(t, "g"))
	if groupID == "" {
		return
	}

	addPatch := buildPatch([]map[string]interface{}{
		{
			"op":     opAdd,
			pathAttr: membersAttr,
			valueAttr: []interface{}{
				map[string]interface{}{
					valueAttr: userID,
				},
			},
		},
	})

	addResp := doRequest(t, cfg, http.MethodPatch, fmt.Sprintf("/Groups/%s", groupID), addPatch)
	if addResp.StatusCode != http.StatusOK && addResp.StatusCode != http.StatusNoContent {
		data, _ := readBody(addResp)
		t.Fatalf("PATCH add member /Groups/%s: expected 200 or 204, got %d\nbody: %s", groupID, addResp.StatusCode, data)
	}
	drainAndClose(addResp)

	getResp := doRequest(t, cfg, http.MethodGet, fmt.Sprintf("/Groups/%s", groupID), nil)
	if getResp.StatusCode != http.StatusOK {
		drainAndClose(getResp)
		t.Fatalf("GET /Groups/%s after add member: expected 200, got %d", groupID, getResp.StatusCode)
	}
	var groupBody map[string]interface{}
	mustDecodeJSON(t, getResp, &groupBody)

	memberFound := false
	members, _ := groupBody[membersAttr].([]interface{})
	for _, raw := range members {
		m, _ := raw.(map[string]interface{})
		if v, _ := m[valueAttr].(string); v == userID {
			memberFound = true
			break
		}
	}
	if !memberFound {
		t.Errorf("user %q not found in group members after PATCH add", userID)
	}

	removePatch := buildPatch([]map[string]interface{}{
		{
			"op":   "remove",
			"path": fmt.Sprintf("members[value eq %q]", userID),
		},
	})

	removeResp := doRequest(t, cfg, http.MethodPatch, fmt.Sprintf("/Groups/%s", groupID), removePatch)
	if removeResp.StatusCode != http.StatusOK && removeResp.StatusCode != http.StatusNoContent {
		data, _ := readBody(removeResp)

		t.Logf("PATCH remove member /Groups/%s: got %d (server may not support value-filter remove)\nbody: %s", groupID, removeResp.StatusCode, data)
		return
	}
	drainAndClose(removeResp)

	getResp2 := doRequest(t, cfg, http.MethodGet, fmt.Sprintf("/Groups/%s", groupID), nil)
	var groupBody2 map[string]interface{}
	mustDecodeJSON(t, getResp2, &groupBody2)

	members2, _ := groupBody2[membersAttr].([]interface{})
	for _, raw := range members2 {
		m, _ := raw.(map[string]interface{})
		if v, _ := m[valueAttr].(string); v == userID {
			t.Errorf("user %q still present in group members after PATCH remove", userID)
			break
		}
	}
}

func testDeleteGroup(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	resp := doRequest(t, cfg, http.MethodPost, "/Groups", NewTestGroup(uniqueSuffix(t, "")))
	if resp.StatusCode != http.StatusCreated {
		drainAndClose(resp)
		t.Skipf("testDeleteGroup: POST /Groups returned %d", resp.StatusCode)
		return
	}
	var created map[string]interface{}
	mustDecodeJSON(t, resp, &created)
	id := checkID(t, created)
	if id == "" {
		return
	}

	del := doRequest(t, cfg, http.MethodDelete, fmt.Sprintf("/Groups/%s", id), nil)
	drainAndClose(del)
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /Groups/%s: expected 204, got %d", id, del.StatusCode)
	}

	get := doRequest(t, cfg, http.MethodGet, fmt.Sprintf("/Groups/%s", id), nil)
	if get.StatusCode != http.StatusNotFound {
		drainAndClose(get)
		t.Errorf("GET after DELETE /Groups/%s: expected 404, got %d", id, get.StatusCode)
		return
	}
	checkErrorResponse(t, get, http.StatusNotFound)
}
