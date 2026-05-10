package schema

import (
	"sync"
	"testing"
)

func makeSchema(id, name string) *Schema {
	return &Schema{ID: id, Name: name}
}

func makeResourceType(name, endpoint, schemaURI string) *ResourceType {
	return &ResourceType{Name: name, Endpoint: endpoint, Schema: schemaURI}
}

func TestRegisterSchema_BasicLookup(t *testing.T) {
	r := NewRegistry()
	s := makeSchema("urn:test:Foo", "Foo")
	r.RegisterSchema(s)

	got, ok := r.SchemaByID("urn:test:Foo")
	if !ok || got == nil {
		t.Fatal("expected to find schema by exact ID")
	}
	if got.Name != "Foo" {
		t.Errorf("unexpected schema name: %q", got.Name)
	}
}

func TestSchemaByID_CaseInsensitive(t *testing.T) {
	r := NewRegistry()
	r.RegisterSchema(makeSchema("urn:test:Schema1", "S1"))

	variants := []string{"urn:test:schema1", "URN:TEST:SCHEMA1", "Urn:Test:Schema1"}
	for _, v := range variants {
		got, ok := r.SchemaByID(v)
		if !ok || got == nil {
			t.Errorf("SchemaByID(%q) not found", v)
		}
	}
}

func TestSchemaByID_NotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.SchemaByID("urn:does:not:exist")
	if ok {
		t.Error("expected ok=false for unknown ID")
	}
}

func TestRegisterSchema_Overwrite(t *testing.T) {
	r := NewRegistry()
	r.RegisterSchema(makeSchema("urn:test:X", "First"))
	r.RegisterSchema(makeSchema("urn:test:X", "Second"))

	got, ok := r.SchemaByID("urn:test:X")
	if !ok {
		t.Fatal("schema not found after overwrite")
	}
	if got.Name != "Second" {
		t.Errorf("expected overwritten name %q, got %q", "Second", got.Name)
	}
}

func TestRegisterResourceType_BasicLookup(t *testing.T) {
	r := NewRegistry()
	rt := makeResourceType("Widget", "/Widgets", "urn:test:Widget")
	r.RegisterResourceType(rt)

	got, ok := r.ResourceTypeByName("Widget")
	if !ok || got == nil {
		t.Fatal("expected to find resource type by name")
	}
	if got.Endpoint != "/Widgets" {
		t.Errorf("unexpected endpoint: %q", got.Endpoint)
	}
}

func TestResourceTypeByName_CaseInsensitive(t *testing.T) {
	r := NewRegistry()
	r.RegisterResourceType(makeResourceType("User", "/Users", UserSchemaURI))

	variants := []string{"user", "USER", "uSeR"}
	for _, v := range variants {
		got, ok := r.ResourceTypeByName(v)
		if !ok || got == nil {
			t.Errorf("ResourceTypeByName(%q) not found", v)
		}
	}
}

func TestResourceTypeByName_NotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.ResourceTypeByName("Unknown")
	if ok {
		t.Error("expected ok=false for unknown resource type")
	}
}

func TestResourceTypeByEndpoint_Found(t *testing.T) {
	r := NewRegistry()
	r.RegisterResourceType(makeResourceType("User", "/Users", UserSchemaURI))

	got, ok := r.ResourceTypeByEndpoint("/Users")
	if !ok || got == nil {
		t.Fatal("expected to find resource type by endpoint")
	}
	if got.Name != "User" {
		t.Errorf("unexpected name: %q", got.Name)
	}
}

func TestResourceTypeByEndpoint_CaseInsensitive(t *testing.T) {
	r := NewRegistry()
	r.RegisterResourceType(makeResourceType("Group", "/Groups", GroupSchemaURI))

	got, ok := r.ResourceTypeByEndpoint("/groups")
	if !ok || got == nil {
		t.Error("ResourceTypeByEndpoint should be case-insensitive")
	}
}

func TestResourceTypeByEndpoint_NotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.ResourceTypeByEndpoint("/NoSuchEndpoint")
	if ok {
		t.Error("expected ok=false for unknown endpoint")
	}
}

func TestSchemas_ReturnsCopy(t *testing.T) {
	r := NewRegistry()
	r.RegisterSchema(makeSchema("urn:a", "A"))
	r.RegisterSchema(makeSchema("urn:b", "B"))

	schemas := r.Schemas()
	if len(schemas) != 2 {
		t.Errorf("expected 2 schemas, got %d", len(schemas))
	}

	schemas[0] = nil
	if s, _ := r.SchemaByID("urn:a"); s == nil {
		t.Error("registry was unexpectedly mutated")
	}
}

func TestResourceTypes_ReturnsCopy(t *testing.T) {
	r := NewRegistry()
	r.RegisterResourceType(makeResourceType("User", "/Users", UserSchemaURI))
	r.RegisterResourceType(makeResourceType("Group", "/Groups", GroupSchemaURI))

	rts := r.ResourceTypes()
	if len(rts) != 2 {
		t.Errorf("expected 2 resource types, got %d", len(rts))
	}
}

func TestRegisterDefaults(t *testing.T) {
	r := NewRegistry()
	r.RegisterDefaults()

	for _, uri := range []string{UserSchemaURI, GroupSchemaURI, EnterpriseUserSchemaURI} {
		if _, ok := r.SchemaByID(uri); !ok {
			t.Errorf("schema %q not registered after RegisterDefaults", uri)
		}
	}

	for _, name := range []string{"User", "Group"} {
		if _, ok := r.ResourceTypeByName(name); !ok {
			t.Errorf("resource type %q not registered after RegisterDefaults", name)
		}
	}

	for _, ep := range []string{"/Users", "/Groups"} {
		if _, ok := r.ResourceTypeByEndpoint(ep); !ok {
			t.Errorf("endpoint %q not registered after RegisterDefaults", ep)
		}
	}
}

func TestRegisterDefaults_Idempotent(t *testing.T) {
	r := NewRegistry()
	r.RegisterDefaults()
	r.RegisterDefaults()

	if got := len(r.Schemas()); got != 3 {
		t.Errorf("expected 3 schemas after double RegisterDefaults, got %d", got)
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	r.RegisterDefaults()

	var wg sync.WaitGroup
	const goroutines = 50

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.SchemaByID(UserSchemaURI)
			r.ResourceTypeByName("User")
			r.ResourceTypeByEndpoint("/Users")
			r.Schemas()
			r.ResourceTypes()
		}()
	}

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r.RegisterSchema(makeSchema("urn:concurrent:"+string(rune('A'+i%26)), "Concurrent"))
		}(i)
	}

	wg.Wait()
}
