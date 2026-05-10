package resource

import (
	"testing"
)

func TestGet_Simple(t *testing.T) {
	r := &Resource{
		Attributes: map[string]interface{}{"userName": "bjensen"},
	}
	val, ok := Get(r, AttributePath{AttributeName: "userName"})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if val != "bjensen" {
		t.Errorf("expected bjensen, got %v", val)
	}
}

func TestGet_CaseInsensitive(t *testing.T) {
	r := &Resource{
		Attributes: map[string]interface{}{"userName": "bjensen"},
	}
	val, ok := Get(r, AttributePath{AttributeName: "USERNAME"})
	if !ok {
		t.Fatal("expected ok=true for case-insensitive lookup")
	}
	if val != "bjensen" {
		t.Errorf("expected bjensen, got %v", val)
	}
}

func TestGet_SubAttribute(t *testing.T) {
	r := &Resource{
		Attributes: map[string]interface{}{
			"name": map[string]interface{}{
				"familyName": "Jensen",
				"givenName":  "Barbara",
			},
		},
	}
	val, ok := Get(r, AttributePath{AttributeName: "name", SubAttribute: "familyName"})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if val != "Jensen" {
		t.Errorf("expected Jensen, got %v", val)
	}
}

func TestGet_SubAttributeMissing(t *testing.T) {
	r := &Resource{
		Attributes: map[string]interface{}{
			"name": map[string]interface{}{"givenName": "Barbara"},
		},
	}
	_, ok := Get(r, AttributePath{AttributeName: "name", SubAttribute: "familyName"})
	if ok {
		t.Error("expected ok=false for missing sub-attribute")
	}
}

func TestGet_Nil(t *testing.T) {
	_, ok := Get(nil, AttributePath{AttributeName: "userName"})
	if ok {
		t.Error("expected ok=false for nil resource")
	}
}

func TestGet_Missing(t *testing.T) {
	r := &Resource{Attributes: map[string]interface{}{}}
	_, ok := Get(r, AttributePath{AttributeName: "nonexistent"})
	if ok {
		t.Error("expected ok=false for missing attribute")
	}
}

func TestSet_Simple(t *testing.T) {
	r := &Resource{Attributes: map[string]interface{}{}}
	Set(r, AttributePath{AttributeName: "userName"}, "jsmith")
	if r.Attributes["userName"] != "jsmith" {
		t.Errorf("expected userName=jsmith, got %v", r.Attributes["userName"])
	}
}

func TestSet_SubAttribute_Create(t *testing.T) {
	r := &Resource{Attributes: map[string]interface{}{}}
	Set(r, AttributePath{AttributeName: "name", SubAttribute: "familyName"}, "Smith")
	nameMap, ok := r.Attributes["name"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map for name, got %T", r.Attributes["name"])
	}
	if nameMap["familyName"] != "Smith" {
		t.Errorf("expected familyName=Smith, got %v", nameMap["familyName"])
	}
}

func TestSet_SubAttribute_Update(t *testing.T) {
	r := &Resource{
		Attributes: map[string]interface{}{
			"name": map[string]interface{}{"familyName": "Old", "givenName": "Barbara"},
		},
	}
	Set(r, AttributePath{AttributeName: "name", SubAttribute: "familyName"}, "New")
	nameMap := r.Attributes["name"].(map[string]interface{})
	if nameMap["familyName"] != "New" {
		t.Errorf("expected familyName=New, got %v", nameMap["familyName"])
	}

	if nameMap["givenName"] != "Barbara" {
		t.Errorf("givenName should be preserved, got %v", nameMap["givenName"])
	}
}

func TestSet_NilResource(t *testing.T) {

	Set(nil, AttributePath{AttributeName: "userName"}, "test")
}

func TestDelete_Simple(t *testing.T) {
	r := &Resource{
		Attributes: map[string]interface{}{"userName": "bjensen", "title": "Engineer"},
	}
	Delete(r, AttributePath{AttributeName: "title"})
	if _, ok := r.Attributes["title"]; ok {
		t.Error("expected title to be deleted")
	}
	if r.Attributes["userName"] != "bjensen" {
		t.Error("userName should not be affected")
	}
}

func TestDelete_SubAttribute(t *testing.T) {
	r := &Resource{
		Attributes: map[string]interface{}{
			"name": map[string]interface{}{"familyName": "Jensen", "givenName": "Barbara"},
		},
	}
	Delete(r, AttributePath{AttributeName: "name", SubAttribute: "familyName"})
	nameMap := r.Attributes["name"].(map[string]interface{})
	if _, ok := nameMap["familyName"]; ok {
		t.Error("expected familyName to be deleted")
	}
	if nameMap["givenName"] != "Barbara" {
		t.Error("givenName should not be affected")
	}
}

func TestDelete_Missing(t *testing.T) {
	r := &Resource{Attributes: map[string]interface{}{}}

	Delete(r, AttributePath{AttributeName: "nonexistent"})
}

func TestDelete_Nil(t *testing.T) {

	Delete(nil, AttributePath{AttributeName: "userName"})
}

func TestGet_ExtensionAttribute(t *testing.T) {

	r := &Resource{
		Attributes: map[string]interface{}{
			"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User": map[string]interface{}{
				"department": "Engineering",
			},
		},
	}
	extPath := AttributePath{
		Schema:        "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User",
		AttributeName: "department",
	}

	val, ok := Get(r, AttributePath{AttributeName: "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"})
	if !ok {
		t.Fatal("expected to find extension namespace")
	}
	extMap, ok := val.(map[string]interface{})
	if !ok {
		t.Fatal("expected map for extension namespace")
	}
	if extMap["department"] != "Engineering" {
		t.Errorf("expected department=Engineering, got %v", extMap["department"])
	}
	_ = extPath
}
