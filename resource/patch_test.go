package resource

import (
	"errors"
	"testing"
)

func baseUser() *Resource {
	return &Resource{
		Schemas: []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		ID:      "user1",
		Attributes: map[string]interface{}{
			"userName": "bjensen",
			"active":   true,
			"emails": []interface{}{
				map[string]interface{}{"value": "home@example.com", "type": "home"},
				map[string]interface{}{"value": "work@example.com", "type": "work", "primary": true},
			},
			"name": map[string]interface{}{
				"familyName": "Jensen",
				"givenName":  "Barbara",
			},
		},
	}
}

func makeOp(op PatchOpType, path string, value interface{}) PatchOp {
	if path == "" {
		return PatchOp{Op: op, Value: value}
	}
	pp, err := ParsePatchPath(path)
	if err != nil {
		panic("makeOp: " + err.Error())
	}
	return PatchOp{Op: op, Path: pp, Value: value}
}

func TestApplyPatch_Add_NoPath_ObjectMerge(t *testing.T) {
	r := baseUser()
	ops := []PatchOp{
		{Op: PatchOpAdd, Value: map[string]interface{}{"title": "Developer", "locale": "en-US"}},
	}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Attributes["title"] != "Developer" {
		t.Errorf("expected title=Developer, got %v", result.Attributes["title"])
	}
	if result.Attributes["locale"] != "en-US" {
		t.Errorf("expected locale=en-US")
	}
}

func TestApplyPatch_Add_NoPath_NotObject(t *testing.T) {
	r := baseUser()
	ops := []PatchOp{
		{Op: PatchOpAdd, Value: "invalid"},
	}
	_, err := ApplyPatch(r, ops)
	if err == nil {
		t.Error("expected error for non-object add without path")
	}
}

func TestApplyPatch_Add_SingleValued(t *testing.T) {
	r := baseUser()
	ops := []PatchOp{makeOp(PatchOpAdd, "title", "Engineer")}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Attributes["title"] != "Engineer" {
		t.Errorf("expected title=Engineer, got %v", result.Attributes["title"])
	}
}

func TestApplyPatch_Add_MultiValued_Append(t *testing.T) {
	r := baseUser()
	newEmail := map[string]interface{}{"value": "other@example.com", "type": "other"}
	ops := []PatchOp{makeOp(PatchOpAdd, "emails", newEmail)}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	emails, _ := toSlice(result.Attributes["emails"])
	if len(emails) != 3 {
		t.Errorf("expected 3 emails after append, got %d", len(emails))
	}
}

func TestApplyPatch_Remove_NoPath_Error(t *testing.T) {
	r := baseUser()
	ops := []PatchOp{{Op: PatchOpRemove}}
	_, err := ApplyPatch(r, ops)
	if !errors.Is(err, ErrNoTarget) {
		t.Errorf("expected ErrNoTarget, got %v", err)
	}
}

func TestApplyPatch_Remove_SingleValued(t *testing.T) {
	r := baseUser()
	r.Attributes["title"] = "Engineer"
	ops := []PatchOp{makeOp(PatchOpRemove, "title", nil)}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := result.Attributes["title"]; ok {
		t.Error("expected title to be removed")
	}
}

func TestApplyPatch_Remove_MultiValued_NoFilter(t *testing.T) {
	r := baseUser()
	ops := []PatchOp{makeOp(PatchOpRemove, "emails", nil)}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := result.Attributes["emails"]; ok {
		t.Error("expected emails to be removed entirely")
	}
}

func TestApplyPatch_Remove_WithValueFilter(t *testing.T) {
	r := baseUser()
	ops := []PatchOp{makeOp(PatchOpRemove, `emails[type eq "home"]`, nil)}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	emails, ok := toSlice(result.Attributes["emails"])
	if !ok {
		t.Fatal("emails should still be present")
	}
	if len(emails) != 1 {
		t.Errorf("expected 1 email after removing home, got %d", len(emails))
	}

	if m, ok := emails[0].(map[string]interface{}); ok {
		if m["type"] != "work" {
			t.Errorf("expected remaining email to be work, got %v", m["type"])
		}
	}
}

func TestApplyPatch_Remove_WithFilterAndSubAttr(t *testing.T) {
	r := baseUser()

	ops := []PatchOp{makeOp(PatchOpRemove, `emails[type eq "work"].primary`, nil)}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	emails, _ := toSlice(result.Attributes["emails"])
	for _, e := range emails {
		if m, ok := e.(map[string]interface{}); ok {
			if m["type"] == "work" {
				if _, hasPrimary := m["primary"]; hasPrimary {
					t.Error("expected primary to be removed from work email")
				}
			}
		}
	}
}

func TestApplyPatch_Replace_NoPath(t *testing.T) {
	r := baseUser()
	ops := []PatchOp{
		{Op: PatchOpReplace, Value: map[string]interface{}{"userName": "jsmith", "active": false}},
	}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Attributes["userName"] != "jsmith" {
		t.Errorf("expected userName=jsmith")
	}
	if result.Attributes["active"] != false {
		t.Errorf("expected active=false")
	}
}

func TestApplyPatch_Replace_SingleValued(t *testing.T) {
	r := baseUser()
	ops := []PatchOp{makeOp(PatchOpReplace, "userName", "jsmith")}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Attributes["userName"] != "jsmith" {
		t.Errorf("expected userName=jsmith, got %v", result.Attributes["userName"])
	}
}

func TestApplyPatch_Replace_MultiValued_NoFilter(t *testing.T) {
	r := baseUser()
	newEmails := []interface{}{
		map[string]interface{}{"value": "new@example.com", "type": "work"},
	}
	ops := []PatchOp{makeOp(PatchOpReplace, "emails", newEmails)}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	emails, _ := toSlice(result.Attributes["emails"])
	if len(emails) != 1 {
		t.Errorf("expected 1 email after replace, got %d", len(emails))
	}
}

func TestApplyPatch_Replace_WithValueFilter(t *testing.T) {
	r := baseUser()
	ops := []PatchOp{makeOp(PatchOpReplace, `emails[type eq "home"].value`, "newhome@example.com")}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	emails, _ := toSlice(result.Attributes["emails"])
	for _, e := range emails {
		if m, ok := e.(map[string]interface{}); ok {
			if m["type"] == "home" {
				if m["value"] != "newhome@example.com" {
					t.Errorf("expected value updated, got %v", m["value"])
				}
			}
		}
	}
}

func TestApplyPatch_Replace_NonExistent_TreatedAsAdd(t *testing.T) {
	r := baseUser()
	ops := []PatchOp{makeOp(PatchOpReplace, "title", "Manager")}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Attributes["title"] != "Manager" {
		t.Errorf("expected title=Manager, got %v", result.Attributes["title"])
	}
}

func TestApplyPatch_Atomicity(t *testing.T) {
	r := baseUser()
	originalName := r.Attributes["userName"]

	ops := []PatchOp{
		makeOp(PatchOpAdd, "title", "Engineer"),

		{Op: PatchOpRemove},
	}
	result, err := ApplyPatch(r, ops)
	if err == nil {
		t.Fatal("expected error from invalid second op")
	}
	if result != nil {
		t.Error("expected nil result on error")
	}

	if r.Attributes["userName"] != originalName {
		t.Error("original resource was modified despite error")
	}
	if _, ok := r.Attributes["title"]; ok {
		t.Error("original should not have title after failed patch")
	}
}

func TestApplyPatch_ExtensionAttribute(t *testing.T) {
	r := baseUser()
	extSchema := "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
	ops := []PatchOp{
		{
			Op: PatchOpAdd,
			Value: map[string]interface{}{
				extSchema: map[string]interface{}{"department": "Engineering"},
			},
		},
	}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ext, ok := result.Attributes[extSchema].(map[string]interface{})
	if !ok {
		t.Fatalf("expected extension namespace to be set, got %T", result.Attributes[extSchema])
	}
	if ext["department"] != "Engineering" {
		t.Errorf("expected department=Engineering, got %v", ext["department"])
	}
}

func TestParsePatchRequest_Valid(t *testing.T) {
	body := map[string]interface{}{
		"schemas": []interface{}{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": []interface{}{
			map[string]interface{}{
				"op":    "add",
				"path":  "title",
				"value": "Engineer",
			},
		},
	}
	req, err := ParsePatchRequest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Operations) != 1 {
		t.Errorf("expected 1 operation, got %d", len(req.Operations))
	}
	if req.Operations[0].Op != PatchOpAdd {
		t.Errorf("expected add op")
	}
}

func TestParsePatchRequest_MissingOperations(t *testing.T) {
	body := map[string]interface{}{
		"schemas": []interface{}{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
	}
	_, err := ParsePatchRequest(body)
	if err == nil {
		t.Error("expected error for missing Operations")
	}
}
