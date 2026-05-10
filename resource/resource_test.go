package resource

import (
	"testing"
	"time"
)

func TestToMap_BasicFields(t *testing.T) {
	r := &Resource{
		Schemas:    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		ID:         "1234",
		ExternalID: "ext-001",
		Attributes: map[string]interface{}{
			"userName": "bjensen",
		},
	}
	m := r.ToMap()

	if schemas, ok := m["schemas"].([]string); !ok || len(schemas) != 1 {
		t.Errorf("expected schemas to be []string with 1 element, got %T %v", m["schemas"], m["schemas"])
	}
	if m["id"] != "1234" {
		t.Errorf("expected id=1234, got %v", m["id"])
	}
	if m["externalId"] != "ext-001" {
		t.Errorf("expected externalId=ext-001, got %v", m["externalId"])
	}
	if m["userName"] != "bjensen" {
		t.Errorf("expected userName=bjensen, got %v", m["userName"])
	}
}

func TestToMap_Meta(t *testing.T) {
	created := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	r := &Resource{
		Schemas: []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		Meta: Meta{
			ResourceType: "User",
			Created:      created,
			Location:     "/Users/1234",
			Version:      `W/"abc"`,
		},
		Attributes: map[string]interface{}{},
	}
	m := r.ToMap()
	metaRaw, ok := m["meta"]
	if !ok {
		t.Fatal("expected meta in ToMap output")
	}
	meta, ok := metaRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("meta should be map[string]interface{}, got %T", metaRaw)
	}
	if meta["resourceType"] != "User" {
		t.Errorf("expected resourceType=User, got %v", meta["resourceType"])
	}
	if meta["location"] != "/Users/1234" {
		t.Errorf("expected location=/Users/1234, got %v", meta["location"])
	}
}

func TestToMap_EmptyMeta(t *testing.T) {
	r := &Resource{
		Schemas:    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		Attributes: map[string]interface{}{},
	}
	m := r.ToMap()
	if _, ok := m["meta"]; ok {
		t.Error("expected no meta key when meta is zero value")
	}
}

func TestFromMap_RoundTrip(t *testing.T) {
	created := time.Date(2021, 5, 1, 0, 0, 0, 0, time.UTC)
	r := &Resource{
		Schemas:    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		ID:         "abc",
		ExternalID: "ext-abc",
		Meta: Meta{
			ResourceType: "User",
			Created:      created,
			Location:     "/Users/abc",
		},
		Attributes: map[string]interface{}{
			"userName": "tester",
			"active":   true,
		},
	}

	m := r.ToMap()
	r2 := FromMap(m)

	if r2.ID != r.ID {
		t.Errorf("ID mismatch: got %q, want %q", r2.ID, r.ID)
	}
	if r2.ExternalID != r.ExternalID {
		t.Errorf("ExternalID mismatch: got %q, want %q", r2.ExternalID, r.ExternalID)
	}
	if len(r2.Schemas) != len(r.Schemas) {
		t.Errorf("Schemas length mismatch: got %v, want %v", r2.Schemas, r.Schemas)
	}
	if r2.Meta.ResourceType != r.Meta.ResourceType {
		t.Errorf("Meta.ResourceType mismatch: got %q, want %q", r2.Meta.ResourceType, r.Meta.ResourceType)
	}
}

func TestFromMap_WellKnownKeysExtracted(t *testing.T) {
	m := map[string]interface{}{
		"schemas":    []interface{}{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"id":         "xyz",
		"externalId": "ext-xyz",
		"userName":   "john",
	}
	r := FromMap(m)
	if r.ID != "xyz" {
		t.Errorf("ID: got %q", r.ID)
	}
	if r.ExternalID != "ext-xyz" {
		t.Errorf("ExternalID: got %q", r.ExternalID)
	}
	if _, ok := r.Attributes["userName"]; !ok {
		t.Error("userName should be in Attributes")
	}
	if _, ok := r.Attributes["id"]; ok {
		t.Error("id should NOT be in Attributes")
	}
}

func TestClone(t *testing.T) {
	r := &Resource{
		Schemas: []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		ID:      "original",
		Attributes: map[string]interface{}{
			"userName": "original",
			"emails": []interface{}{
				map[string]interface{}{"value": "orig@example.com", "primary": true},
			},
		},
	}

	c := r.Clone()
	if c == r {
		t.Fatal("Clone returned same pointer")
	}

	c.ID = "modified"
	c.Attributes["userName"] = "modified"

	if r.ID != "original" {
		t.Errorf("original ID changed after modifying clone")
	}
	if r.Attributes["userName"] != "original" {
		t.Errorf("original userName changed after modifying clone")
	}
}

func TestClone_Nil(t *testing.T) {
	var r *Resource
	if got := r.Clone(); got != nil {
		t.Errorf("Clone of nil should return nil, got %v", got)
	}
}
