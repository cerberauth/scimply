package schema

import "testing"

func newTestSchema() *Schema {
	return &Schema{
		ID:   "urn:test:schema",
		Name: "Test",
		Attributes: []Attribute{
			{Name: "userName", Type: TypeString},
			{Name: "displayName", Type: TypeString},
			{Name: "active", Type: TypeBoolean},
		},
	}
}

func TestAttributeByName_Found(t *testing.T) {
	s := newTestSchema()
	attr := s.AttributeByName("userName")
	if attr == nil {
		t.Fatal("expected to find attribute 'userName', got nil")
	}
	if attr.Name != "userName" {
		t.Errorf("unexpected attribute name: %q", attr.Name)
	}
}

func TestAttributeByName_CaseInsensitive(t *testing.T) {
	s := newTestSchema()
	variants := []string{"username", "USERNAME", "UserName", "uSeRnAmE"}
	for _, v := range variants {
		attr := s.AttributeByName(v)
		if attr == nil {
			t.Errorf("AttributeByName(%q) returned nil, want non-nil", v)
		}
	}
}

func TestAttributeByName_NotFound(t *testing.T) {
	s := newTestSchema()
	attr := s.AttributeByName("nonExistent")
	if attr != nil {
		t.Errorf("expected nil for unknown attribute, got %+v", attr)
	}
}

func TestAttributeByName_EmptySchema(t *testing.T) {
	s := &Schema{}
	attr := s.AttributeByName("anything")
	if attr != nil {
		t.Errorf("expected nil for empty schema, got %+v", attr)
	}
}

func TestAttributeByName_ReturnsPointerIntoSlice(t *testing.T) {
	s := newTestSchema()

	attr := s.AttributeByName("active")
	if attr == nil {
		t.Fatal("expected non-nil")
	}
	attr.Description = "mutated"
	if s.Attributes[2].Description != "mutated" {
		t.Error("expected pointer to refer into the original slice")
	}
}
