package v1

import (
	"testing"
)

func TestToV2_SchemaURIConversion(t *testing.T) {
	v1Resource := map[string]interface{}{
		"schemas":  []interface{}{CoreSchemaURI},
		"id":       "abc-123",
		"userName": "bjensen",
	}

	res, err := ToV2(v1Resource, "User")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Schemas) == 0 {
		t.Fatal("expected at least one schema URI")
	}
	found := false
	for _, s := range res.Schemas {
		if s == v2UserSchemaURI {
			found = true
		}
		if s == CoreSchemaURI {
			t.Errorf("v1 schema URI should not appear in v2 resource, got %q", s)
		}
	}
	if !found {
		t.Errorf("expected v2 user schema URI %q in schemas %v", v2UserSchemaURI, res.Schemas)
	}
}

func TestToV2_GroupSchemaURI(t *testing.T) {
	v1Resource := map[string]interface{}{
		"schemas":     []interface{}{CoreSchemaURI},
		"id":          "grp-1",
		"displayName": "Admins",
	}

	res, err := ToV2(v1Resource, "Group")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, s := range res.Schemas {
		if s == v2GroupSchemaURI {
			found = true
		}
	}
	if !found {
		t.Errorf("expected v2 group schema URI in schemas %v", res.Schemas)
	}
}

func TestToV2_EnterpriseExtension(t *testing.T) {
	v1Resource := map[string]interface{}{
		"schemas":        []interface{}{CoreSchemaURI, EnterpriseSchemaURI},
		"userName":       "bjensen",
		"employeeNumber": "12345",
		"department":     "Engineering",
		"costCenter":     "CC-001",
	}

	res, err := ToV2(v1Resource, "User")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ext, ok := res.Attributes[v2EnterpriseSchemaURI].(map[string]interface{})
	if !ok {
		t.Fatalf("expected enterprise extension under key %q, got %T", v2EnterpriseSchemaURI, res.Attributes[v2EnterpriseSchemaURI])
	}
	if ext["employeeNumber"] != "12345" {
		t.Errorf("employeeNumber: got %v, want 12345", ext["employeeNumber"])
	}
	if ext["department"] != "Engineering" {
		t.Errorf("department: got %v, want Engineering", ext["department"])
	}

	if _, ok := res.Attributes["employeeNumber"]; ok {
		t.Error("employeeNumber should not be at resource top level after v1→v2 conversion")
	}

	foundExt := false
	for _, s := range res.Schemas {
		if s == v2EnterpriseSchemaURI {
			foundExt = true
		}
	}
	if !foundExt {
		t.Errorf("expected enterprise extension URI in schemas %v", res.Schemas)
	}
}

func TestToV2_ManagerIdToValue(t *testing.T) {
	v1Resource := map[string]interface{}{
		"schemas":  []interface{}{CoreSchemaURI, EnterpriseSchemaURI},
		"userName": "bjensen",
		"manager": map[string]interface{}{
			"managerId":   "mgr-456",
			"displayName": "John Smith",
		},
	}

	res, err := ToV2(v1Resource, "User")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ext, ok := res.Attributes[v2EnterpriseSchemaURI].(map[string]interface{})
	if !ok {
		t.Fatal("expected enterprise extension")
	}
	mgr, ok := ext["manager"].(map[string]interface{})
	if !ok {
		t.Fatal("expected manager as map")
	}

	if mgr["value"] != "mgr-456" {
		t.Errorf("manager.value: got %v, want mgr-456", mgr["value"])
	}
	if _, exists := mgr["managerId"]; exists {
		t.Error("managerId should have been renamed to value")
	}
}

func TestFromV2_SchemaURIConversion(t *testing.T) {
	v1Resource := map[string]interface{}{
		"schemas":  []interface{}{CoreSchemaURI},
		"userName": "bjensen",
	}
	res, err := ToV2(v1Resource, "User")
	if err != nil {
		t.Fatalf("ToV2 error: %v", err)
	}

	out, err := FromV2(res, "User")
	if err != nil {
		t.Fatalf("FromV2 error: %v", err)
	}

	schemas, ok := toStringSlice(out["schemas"])
	if !ok || len(schemas) == 0 {
		t.Fatal("expected schemas in v1 output")
	}
	found := false
	for _, s := range schemas {
		if s == CoreSchemaURI {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %q in v1 schemas %v", CoreSchemaURI, schemas)
	}
	for _, s := range schemas {
		if s == v2UserSchemaURI || s == v2GroupSchemaURI {
			t.Errorf("v2 schema URI %q should not appear in v1 output", s)
		}
	}
}

func TestFromV2_EnterpriseExtensionFlattened(t *testing.T) {
	v1Input := map[string]interface{}{
		"schemas":        []interface{}{CoreSchemaURI, EnterpriseSchemaURI},
		"userName":       "bjensen",
		"employeeNumber": "12345",
	}
	res, err := ToV2(v1Input, "User")
	if err != nil {
		t.Fatalf("ToV2 error: %v", err)
	}
	out, err := FromV2(res, "User")
	if err != nil {
		t.Fatalf("FromV2 error: %v", err)
	}

	if out["employeeNumber"] != "12345" {
		t.Errorf("employeeNumber: got %v, want 12345", out["employeeNumber"])
	}

	if _, exists := out[v2EnterpriseSchemaURI]; exists {
		t.Error("extension URI key should not appear in v1 output")
	}
}

func TestFilterV1ToV2_Passthrough(t *testing.T) {
	filters := []string{
		`userName eq "bjensen"`,
		`name.familyName co "Jensen"`,
		`active eq true`,
		`meta.lastModified gt "2023-01-01T00:00:00Z"`,
	}
	for _, f := range filters {
		if got := FilterV1ToV2(f); got != f {
			t.Errorf("FilterV1ToV2(%q) = %q, want %q", f, got, f)
		}
	}
}
