package resource

import (
	"testing"
	"time"
)

func TestEvalFilter_Gt_Number(t *testing.T) {
	r := &Resource{
		Attributes: map[string]interface{}{"score": float64(42)},
	}
	tests := []struct {
		filter string
		want   bool
	}{
		{`score gt 40`, true},
		{`score ge 42`, true},
		{`score ge 43`, false},
		{`score lt 50`, true},
		{`score le 42`, true},
		{`score le 41`, false},
		{`score gt 42`, false},
		{`score lt 42`, false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.filter, func(t *testing.T) {
			expr, err := ParseFilter(tc.filter)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := EvalFilter(expr, r)
			if got != tc.want {
				t.Errorf("EvalFilter(%q) = %v, want %v", tc.filter, got, tc.want)
			}
		})
	}
}

func TestEvalFilter_Gt_DateTime(t *testing.T) {
	ts := time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	r := &Resource{
		Attributes: map[string]interface{}{
			"meta": map[string]interface{}{
				"lastModified": ts,
			},
		},
	}
	if !EvalFilter(mustParse(`meta.lastModified gt "2023-01-01T00:00:00Z"`), r) {
		t.Error("expected gt match for datetime")
	}
	if EvalFilter(mustParse(`meta.lastModified lt "2023-01-01T00:00:00Z"`), r) {
		t.Error("expected no lt match for datetime")
	}
	if !EvalFilter(mustParse(`meta.lastModified ge "`+ts+`"`), r) {
		t.Error("expected ge match for equal datetime")
	}
	if !EvalFilter(mustParse(`meta.lastModified le "`+ts+`"`), r) {
		t.Error("expected le match for equal datetime")
	}
}

func TestEvalFilter_Ne_Extra(t *testing.T) {
	r := &Resource{
		Attributes: map[string]interface{}{"userName": "bjensen"},
	}
	if !EvalFilter(mustParse(`userName ne "jsmith"`), r) {
		t.Error("expected ne match")
	}
	if EvalFilter(mustParse(`userName ne "bjensen"`), r) {
		t.Error("expected no ne match when equal")
	}
}

func TestEvalFilter_Co_Ew(t *testing.T) {
	r := &Resource{
		Attributes: map[string]interface{}{"email": "user@example.com"},
	}
	if !EvalFilter(mustParse(`email co "@example"`), r) {
		t.Error("expected co match")
	}
	if !EvalFilter(mustParse(`email ew ".com"`), r) {
		t.Error("expected ew match")
	}
	if EvalFilter(mustParse(`email ew ".org"`), r) {
		t.Error("expected no ew match")
	}
}

func TestEvalFilter_Null(t *testing.T) {
	r := &Resource{
		Attributes: map[string]interface{}{"nickName": nil},
	}
	if !EvalFilter(mustParse(`nickName eq null`), r) {
		t.Error("expected eq null match")
	}
	if EvalFilter(mustParse(`nickName ne null`), r) {
		t.Error("expected no ne null match")
	}
}

func TestEvalFilter_MultiValued_AnyMatch(t *testing.T) {
	r := &Resource{
		Attributes: map[string]interface{}{
			"emails": []interface{}{
				map[string]interface{}{"value": "home@test.com", "type": "home"},
				map[string]interface{}{"value": "work@example.com", "type": "work"},
			},
		},
	}

	if !EvalFilter(mustParse(`emails.type eq "work"`), r) {
		t.Error("expected match on multi-valued emails.type")
	}

	if !EvalFilter(mustParse(`emails.value co "@example"`), r) {
		t.Error("expected co match on emails.value")
	}
}

func TestEvalFilter_ValuePath_And(t *testing.T) {
	r := &Resource{
		Attributes: map[string]interface{}{
			"emails": []interface{}{
				map[string]interface{}{"value": "work@example.com", "type": "work"},
				map[string]interface{}{"value": "home@test.com", "type": "home"},
			},
		},
	}
	expr := mustParse(`emails[type eq "work" and value co "@example.com"]`)
	if !EvalFilter(expr, r) {
		t.Error("expected value path filter to match")
	}

	expr2 := mustParse(`emails[type eq "home" and value co "@example.com"]`)
	if EvalFilter(expr2, r) {
		t.Error("expected value path filter to NOT match")
	}
}

func TestEvalFilter_NilResource_Extra(t *testing.T) {
	expr := mustParse(`userName eq "bjensen"`)
	if EvalFilter(expr, nil) {
		t.Error("expected false for nil resource")
	}
}

func TestEvalFilter_EmptyAttributes(t *testing.T) {
	r := &Resource{Attributes: map[string]interface{}{}}
	expr := mustParse(`userName pr`)
	if EvalFilter(expr, r) {
		t.Error("expected pr to be false for absent attr")
	}
}

func TestApplyPatch_Add_MultiValued_AppendExtra(t *testing.T) {
	r := baseUser()
	ops := []PatchOp{
		makeOp(PatchOpAdd, "emails", []interface{}{
			map[string]interface{}{"value": "new@example.com", "type": "other"},
		}),
	}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	emails, ok := result.Attributes["emails"].([]interface{})
	if !ok {
		t.Fatal("emails is not a slice")
	}
	if len(emails) != 3 {
		t.Errorf("expected 3 emails after append, got %d", len(emails))
	}
}

func TestApplyPatch_Add_WithValueFilter(t *testing.T) {
	r := baseUser()

	ops := []PatchOp{
		makeOp(PatchOpAdd, `emails[type eq "work"].display`, "Work Email"),
	}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	emails := result.Attributes["emails"].([]interface{})
	for _, e := range emails {
		m := e.(map[string]interface{})
		if m["type"] == "work" {
			if m["display"] != "Work Email" {
				t.Errorf("display: got %v, want Work Email", m["display"])
			}
			return
		}
	}
	t.Error("work email not found after patch")
}

func TestApplyPatch_Replace_WithValueFilter_SubAttr(t *testing.T) {
	r := baseUser()
	ops := []PatchOp{
		makeOp(PatchOpReplace, `emails[type eq "work"].value`, "newwork@example.com"),
	}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	emails := result.Attributes["emails"].([]interface{})
	for _, e := range emails {
		m := e.(map[string]interface{})
		if m["type"] == "work" {
			if m["value"] != "newwork@example.com" {
				t.Errorf("value: got %v, want newwork@example.com", m["value"])
			}
			return
		}
	}
	t.Error("work email not found after patch")
}

func TestApplyPatch_Replace_MultiValued_NoFilter_ReplaceAll(t *testing.T) {
	r := baseUser()
	newEmails := []interface{}{
		map[string]interface{}{"value": "only@example.com", "type": "work"},
	}
	ops := []PatchOp{makeOp(PatchOpReplace, "emails", newEmails)}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	emails := result.Attributes["emails"].([]interface{})
	if len(emails) != 1 {
		t.Errorf("expected 1 email, got %d", len(emails))
	}
}

func TestApplyPatch_Remove_WithValueFilter_Extra(t *testing.T) {
	r := baseUser()
	ops := []PatchOp{
		makeOp(PatchOpRemove, `emails[type eq "home"]`, nil),
	}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	emails := result.Attributes["emails"].([]interface{})
	if len(emails) != 1 {
		t.Errorf("expected 1 email after removing home, got %d", len(emails))
	}
	for _, e := range emails {
		if e.(map[string]interface{})["type"] == "home" {
			t.Error("home email should have been removed")
		}
	}
}

func TestApplyPatch_Remove_SubAttr_WithFilter(t *testing.T) {
	r := baseUser()

	ops := []PatchOp{
		makeOp(PatchOpRemove, `emails[type eq "work"].primary`, nil),
	}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	emails := result.Attributes["emails"].([]interface{})
	for _, e := range emails {
		m := e.(map[string]interface{})
		if m["type"] == "work" {
			if _, exists := m["primary"]; exists {
				t.Error("primary should have been removed from work email")
			}
			return
		}
	}
}

func TestApplyPatch_Add_SubAttrNoFilter(t *testing.T) {
	r := baseUser()

	ops := []PatchOp{
		makeOp(PatchOpAdd, "name.honorificPrefix", "Ms."),
	}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	name := result.Attributes["name"].(map[string]interface{})
	if name["honorificPrefix"] != "Ms." {
		t.Errorf("honorificPrefix: got %v, want Ms.", name["honorificPrefix"])
	}
}

func TestApplyPatch_Replace_NoPath_NonObject_Error(t *testing.T) {
	r := baseUser()
	ops := []PatchOp{{Op: PatchOpReplace, Path: nil, Value: "not an object"}}
	_, err := ApplyPatch(r, ops)
	if err == nil {
		t.Error("expected error for replace without path with non-object value")
	}
}

func TestApplyPatch_Add_NoPath_NonObject_Error(t *testing.T) {
	r := baseUser()
	ops := []PatchOp{{Op: PatchOpAdd, Path: nil, Value: "not an object"}}
	_, err := ApplyPatch(r, ops)
	if err == nil {
		t.Error("expected error for add without path with non-object value")
	}
}

func TestApplyPatch_Atomicity_SecondOpFails(t *testing.T) {
	r := baseUser()
	origUserName := r.Attributes["userName"]

	ops := []PatchOp{
		makeOp(PatchOpReplace, "userName", "new-name"),

		{Op: PatchOpRemove, Path: nil},
	}
	_, err := ApplyPatch(r, ops)
	if err == nil {
		t.Fatal("expected error from second op")
	}

	if r.Attributes["userName"] != origUserName {
		t.Error("original resource was modified despite patch failure")
	}
}

func TestApplyPatch_ExtensionAttr(t *testing.T) {
	r := baseUser()
	extURI := "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
	ops := []PatchOp{
		makeOp(PatchOpReplace, extURI+":department", "Engineering"),
	}
	result, err := ApplyPatch(r, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path, parseErr := ParsePath(extURI + ":department")
	if parseErr != nil {
		t.Fatalf("parse path: %v", parseErr)
	}
	val, ok := Get(result, path)
	if !ok || val != "Engineering" {
		t.Errorf("extension attr: got %v (ok=%v), want Engineering", val, ok)
	}
}

func mustParse(filter string) FilterExpression {
	expr, err := ParseFilter(filter)
	if err != nil {
		panic("mustParse: " + err.Error())
	}
	return expr
}
