package schema

import "testing"

func findAttr(s *Schema, name string) *Attribute {
	return s.AttributeByName(name)
}

func TestCoreUserSchema_UserName(t *testing.T) {
	s := CoreUserSchema()
	attr := findAttr(s, "userName")
	if attr == nil {
		t.Fatal("userName attribute not found in CoreUserSchema")
	}
	if !attr.Required {
		t.Error("userName should be Required=true")
	}
	if attr.Mutability != MutabilityReadWrite {
		t.Errorf("userName Mutability: got %q, want %q", attr.Mutability, MutabilityReadWrite)
	}
	if attr.Uniqueness != UniquenessServer {
		t.Errorf("userName Uniqueness: got %q, want %q", attr.Uniqueness, UniquenessServer)
	}
}

func TestCoreUserSchema_Password(t *testing.T) {
	s := CoreUserSchema()
	attr := findAttr(s, "password")
	if attr == nil {
		t.Fatal("password attribute not found in CoreUserSchema")
	}
	if attr.Mutability != MutabilityWriteOnly {
		t.Errorf("password Mutability: got %q, want %q", attr.Mutability, MutabilityWriteOnly)
	}
	if attr.Returned != ReturnedNever {
		t.Errorf("password Returned: got %q, want %q", attr.Returned, ReturnedNever)
	}
}

func TestCoreUserSchema_ID(t *testing.T) {
	s := CoreUserSchema()
	attr := findAttr(s, "id")
	if attr == nil {
		t.Fatal("id attribute not found in CoreUserSchema")
	}
	if attr.Mutability != MutabilityReadOnly {
		t.Errorf("id Mutability: got %q, want %q", attr.Mutability, MutabilityReadOnly)
	}
	if attr.Returned != ReturnedAlways {
		t.Errorf("id Returned: got %q, want %q", attr.Returned, ReturnedAlways)
	}
	if !attr.CaseExact {
		t.Error("id should be CaseExact=true")
	}
	if attr.Uniqueness != UniquenessServer {
		t.Errorf("id Uniqueness: got %q, want %q", attr.Uniqueness, UniquenessServer)
	}
}

func TestCoreUserSchema_Groups(t *testing.T) {
	s := CoreUserSchema()
	attr := findAttr(s, "groups")
	if attr == nil {
		t.Fatal("groups attribute not found in CoreUserSchema")
	}
	if attr.Mutability != MutabilityReadOnly {
		t.Errorf("groups Mutability: got %q, want %q", attr.Mutability, MutabilityReadOnly)
	}
	if !attr.MultiValued {
		t.Error("groups should be MultiValued=true")
	}
	if attr.Type != TypeComplex {
		t.Errorf("groups Type: got %q, want %q", attr.Type, TypeComplex)
	}
}

func TestCoreUserSchema_Active(t *testing.T) {
	s := CoreUserSchema()
	attr := findAttr(s, "active")
	if attr == nil {
		t.Fatal("active attribute not found in CoreUserSchema")
	}
	if attr.Type != TypeBoolean {
		t.Errorf("active Type: got %q, want %q", attr.Type, TypeBoolean)
	}
}

func TestCoreUserSchema_Name(t *testing.T) {
	s := CoreUserSchema()
	attr := findAttr(s, "name")
	if attr == nil {
		t.Fatal("name attribute not found in CoreUserSchema")
	}
	if attr.Type != TypeComplex {
		t.Errorf("name Type: got %q, want %q", attr.Type, TypeComplex)
	}

	wantSubs := []string{"formatted", "familyName", "givenName", "middleName", "honorificPrefix", "honorificSuffix"}
	subMap := make(map[string]bool, len(attr.SubAttributes))
	for _, sa := range attr.SubAttributes {
		subMap[sa.Name] = true
	}
	for _, name := range wantSubs {
		if !subMap[name] {
			t.Errorf("name sub-attribute %q not found", name)
		}
	}
}

func TestCoreUserSchema_Emails(t *testing.T) {
	s := CoreUserSchema()
	attr := findAttr(s, "emails")
	if attr == nil {
		t.Fatal("emails attribute not found in CoreUserSchema")
	}
	if !attr.MultiValued {
		t.Error("emails should be MultiValued=true")
	}

	var typeAttr *Attribute
	for i := range attr.SubAttributes {
		if attr.SubAttributes[i].Name == "type" {
			typeAttr = &attr.SubAttributes[i]
			break
		}
	}
	if typeAttr == nil {
		t.Fatal("emails.type sub-attribute not found")
	}
	want := map[string]bool{"work": true, "home": true, "other": true}
	for _, cv := range typeAttr.CanonicalValues {
		if !want[cv] {
			t.Errorf("unexpected canonical value %q for emails.type", cv)
		}
		delete(want, cv)
	}
	for missing := range want {
		t.Errorf("missing canonical value %q for emails.type", missing)
	}
}

func TestCoreUserSchema_URI(t *testing.T) {
	s := CoreUserSchema()
	if s.ID != UserSchemaURI {
		t.Errorf("ID: got %q, want %q", s.ID, UserSchemaURI)
	}
}

func TestCoreGroupSchema_DisplayName(t *testing.T) {
	s := CoreGroupSchema()
	attr := findAttr(s, "displayName")
	if attr == nil {
		t.Fatal("displayName attribute not found in CoreGroupSchema")
	}
	if !attr.Required {
		t.Error("displayName should be Required=true")
	}
}

func TestCoreGroupSchema_Members(t *testing.T) {
	s := CoreGroupSchema()
	attr := findAttr(s, "members")
	if attr == nil {
		t.Fatal("members attribute not found in CoreGroupSchema")
	}
	if !attr.MultiValued {
		t.Error("members should be MultiValued=true")
	}
	if attr.Type != TypeComplex {
		t.Errorf("members Type: got %q, want %q", attr.Type, TypeComplex)
	}
}

func TestEnterpriseUserSchema_Manager(t *testing.T) {
	s := EnterpriseUserSchema()
	if s.ID != EnterpriseUserSchemaURI {
		t.Errorf("ID: got %q, want %q", s.ID, EnterpriseUserSchemaURI)
	}

	attr := findAttr(s, "manager")
	if attr == nil {
		t.Fatal("manager attribute not found in EnterpriseUserSchema")
	}
	if attr.Type != TypeComplex {
		t.Errorf("manager Type: got %q, want %q", attr.Type, TypeComplex)
	}

	var displayName *Attribute
	for i := range attr.SubAttributes {
		if attr.SubAttributes[i].Name == "displayName" {
			displayName = &attr.SubAttributes[i]
			break
		}
	}
	if displayName == nil {
		t.Fatal("manager.displayName sub-attribute not found")
	}
	if displayName.Mutability != MutabilityReadOnly {
		t.Errorf("manager.displayName Mutability: got %q, want %q", displayName.Mutability, MutabilityReadOnly)
	}
}

func TestUserResourceType(t *testing.T) {
	rt := UserResourceType()
	if rt.Endpoint != "/Users" {
		t.Errorf("Endpoint: got %q, want %q", rt.Endpoint, "/Users")
	}
	if rt.Schema != UserSchemaURI {
		t.Errorf("Schema: got %q, want %q", rt.Schema, UserSchemaURI)
	}
	if len(rt.SchemaExtensions) == 0 {
		t.Error("expected at least one SchemaExtension (EnterpriseUser)")
	}
	found := false
	for _, ext := range rt.SchemaExtensions {
		if ext.Schema == EnterpriseUserSchemaURI {
			found = true
		}
	}
	if !found {
		t.Error("EnterpriseUser extension not listed in User ResourceType SchemaExtensions")
	}
}

func TestGroupResourceType(t *testing.T) {
	rt := GroupResourceType()
	if rt.Endpoint != "/Groups" {
		t.Errorf("Endpoint: got %q, want %q", rt.Endpoint, "/Groups")
	}
	if rt.Schema != GroupSchemaURI {
		t.Errorf("Schema: got %q, want %q", rt.Schema, GroupSchemaURI)
	}
}
