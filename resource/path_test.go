package resource

import (
	"testing"
)

func TestParsePath_Simple(t *testing.T) {
	p, err := ParsePath("userName")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.AttributeName != "userName" {
		t.Errorf("expected AttributeName=userName, got %q", p.AttributeName)
	}
	if p.SubAttribute != "" {
		t.Errorf("expected empty SubAttribute, got %q", p.SubAttribute)
	}
	if p.Schema != "" {
		t.Errorf("expected empty Schema, got %q", p.Schema)
	}
}

func TestParsePath_SubAttribute(t *testing.T) {
	p, err := ParsePath("name.familyName")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.AttributeName != "name" {
		t.Errorf("expected AttributeName=name, got %q", p.AttributeName)
	}
	if p.SubAttribute != "familyName" {
		t.Errorf("expected SubAttribute=familyName, got %q", p.SubAttribute)
	}
	if p.Schema != "" {
		t.Errorf("expected empty Schema, got %q", p.Schema)
	}
}

func TestParsePath_SchemaPrefix(t *testing.T) {
	full := "urn:ietf:params:scim:schemas:core:2.0:User:userName"
	p, err := ParsePath(full)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.AttributeName != "userName" {
		t.Errorf("expected AttributeName=userName, got %q", p.AttributeName)
	}
	if p.Schema != "urn:ietf:params:scim:schemas:core:2.0:User" {
		t.Errorf("unexpected Schema: %q", p.Schema)
	}
	if p.SubAttribute != "" {
		t.Errorf("expected empty SubAttribute, got %q", p.SubAttribute)
	}
}

func TestParsePath_Invalid(t *testing.T) {
	cases := []string{
		"",
		".",
		".name",
		"name.",
		"1invalid",
		"name..sub",
	}
	for _, tc := range cases {
		_, err := ParsePath(tc)
		if err == nil {
			t.Errorf("expected error for input %q, got nil", tc)
		}
	}
}

func TestAttributePath_String(t *testing.T) {
	cases := []struct {
		path AttributePath
		want string
	}{
		{AttributePath{AttributeName: "userName"}, "userName"},
		{AttributePath{AttributeName: "name", SubAttribute: "familyName"}, "name.familyName"},
		{AttributePath{Schema: "urn:ietf:params:scim:schemas:core:2.0:User", AttributeName: "userName"}, "urn:ietf:params:scim:schemas:core:2.0:User:userName"},
	}
	for _, tc := range cases {
		got := tc.path.String()
		if got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}

func TestParsePatchPath_NoFilter(t *testing.T) {
	pp, err := ParsePatchPath("emails")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pp.Attribute.AttributeName != "emails" {
		t.Errorf("expected AttributeName=emails, got %q", pp.Attribute.AttributeName)
	}
	if pp.ValueFilter != nil {
		t.Error("expected nil ValueFilter")
	}
	if pp.SubAttribute != "" {
		t.Error("expected empty SubAttribute")
	}
}

func TestParsePatchPath_WithFilter(t *testing.T) {
	pp, err := ParsePatchPath(`emails[type eq "work"]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pp.Attribute.AttributeName != "emails" {
		t.Errorf("expected AttributeName=emails, got %q", pp.Attribute.AttributeName)
	}
	if pp.ValueFilter == nil {
		t.Fatal("expected non-nil ValueFilter")
	}
	if pp.SubAttribute != "" {
		t.Error("expected empty SubAttribute")
	}
}

func TestParsePatchPath_WithFilterAndSubAttr(t *testing.T) {
	pp, err := ParsePatchPath(`emails[type eq "work"].value`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pp.Attribute.AttributeName != "emails" {
		t.Errorf("expected AttributeName=emails, got %q", pp.Attribute.AttributeName)
	}
	if pp.ValueFilter == nil {
		t.Fatal("expected non-nil ValueFilter")
	}
	if pp.SubAttribute != "value" {
		t.Errorf("expected SubAttribute=value, got %q", pp.SubAttribute)
	}
}

func TestParsePatchPath_SubAttrNoFilter(t *testing.T) {
	pp, err := ParsePatchPath("name.familyName")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pp.Attribute.AttributeName != "name" {
		t.Errorf("expected AttributeName=name, got %q", pp.Attribute.AttributeName)
	}
	if pp.Attribute.SubAttribute != "familyName" {
		t.Errorf("expected SubAttribute=familyName in Attribute, got %q", pp.Attribute.SubAttribute)
	}
}
