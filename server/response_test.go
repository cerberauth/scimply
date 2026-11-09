package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/cerberauth/scimply/schema"
	"github.com/cerberauth/scimply/server"
	"github.com/cerberauth/scimply/store"
)

func assertNoPassword(t *testing.T, label string, m map[string]interface{}) {
	t.Helper()
	for k := range m {
		if strings.EqualFold(k, "password") {
			t.Errorf("%s: password must not be returned (RFC 7643 returned=never), got %v", label, m[k])
		}
	}
}

func TestReturnedNever_PasswordNotReturned(t *testing.T) {
	s := newTestServer(t)

	resp := doRequest(t, s, http.MethodPost, "/Users", `{"userName":"alice","password":"s3cret"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created map[string]interface{}
	decodeJSON(t, resp, &created)
	assertNoPassword(t, "create", created)
	id, _ := created["id"].(string)

	resp = doRequest(t, s, http.MethodGet, "/Users/"+id, "")
	var got map[string]interface{}
	decodeJSON(t, resp, &got)
	assertNoPassword(t, "get", got)

	resp = doRequest(t, s, http.MethodPut, "/Users/"+id, `{"userName":"alice","password":"other"}`)
	var replaced map[string]interface{}
	decodeJSON(t, resp, &replaced)
	assertNoPassword(t, "replace", replaced)

	resp = doRequest(t, s, http.MethodPatch, "/Users/"+id,
		`{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"replace","path":"password","value":"x"}]}`)
	var patched map[string]interface{}
	decodeJSON(t, resp, &patched)
	assertNoPassword(t, "patch", patched)

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/Users", ""},
		{http.MethodPost, "/Users/.search", `{"filter":"userName eq \"alice\""}`},
	} {
		resp = doRequest(t, s, tc.method, tc.path, tc.body)
		var list struct {
			Resources []map[string]interface{} `json:"Resources"`
		}
		decodeJSON(t, resp, &list)
		if len(list.Resources) == 0 {
			t.Fatalf("%s %s: expected resources", tc.method, tc.path)
		}
		for _, r := range list.Resources {
			assertNoPassword(t, tc.method+" "+tc.path, r)
		}
	}

	resp = doRequest(t, s, http.MethodPost, "/Bulk",
		`{"schemas":["urn:ietf:params:scim:api:messages:2.0:BulkRequest"],"Operations":[{"method":"POST","path":"/Users","bulkId":"b1","data":{"userName":"bob","password":"pw"}}]}`)
	var bulk struct {
		Operations []struct {
			Response map[string]interface{} `json:"response"`
		} `json:"Operations"`
	}
	decodeJSON(t, resp, &bulk)
	if len(bulk.Operations) != 1 || bulk.Operations[0].Response == nil {
		t.Fatalf("bulk: unexpected response %+v", bulk)
	}
	assertNoPassword(t, "bulk", bulk.Operations[0].Response)
}

func TestReturnedNever_ExtensionAndSubAttributes(t *testing.T) {
	const extURI = "urn:example:params:scim:schemas:extension:test:2.0:User"

	reg := schema.NewRegistry()
	reg.RegisterDefaults()
	reg.RegisterSchema(&schema.Schema{
		ID:   extURI,
		Name: "Test",
		Attributes: []schema.Attribute{
			{Name: "secret", Type: schema.TypeString, Returned: schema.ReturnedNever},
			{Name: "visible", Type: schema.TypeString, Returned: schema.ReturnedDefault},
		},
	})
	rt, _ := reg.ResourceTypeByName("User")
	rtCopy := *rt
	rtCopy.SchemaExtensions = append(append([]schema.SchemaExtension{}, rt.SchemaExtensions...),
		schema.SchemaExtension{Schema: extURI})
	reg.RegisterResourceType(&rtCopy)

	s, err := server.New(server.WithStore(store.NewMemoryStore()), server.WithSchemaRegistry(reg))
	if err != nil {
		t.Fatal(err)
	}

	resp := doRequest(t, s, http.MethodPost, "/Users",
		`{"userName":"carol","`+extURI+`":{"secret":"hidden","visible":"shown"}}`)
	var m map[string]interface{}
	decodeJSON(t, resp, &m)
	ext, ok := m[extURI].(map[string]interface{})
	if !ok {
		t.Fatalf("missing extension in response: %v", m)
	}
	if _, ok := ext["secret"]; ok {
		t.Error("extension attribute with returned=never must not be returned")
	}
	if ext["visible"] != "shown" {
		t.Errorf("extension attribute visible: got %v", ext["visible"])
	}
}

func TestWithPasswordHasher(t *testing.T) {
	ms := store.NewMemoryStore()
	reg := schema.NewRegistry()
	reg.RegisterDefaults()
	s, err := server.New(
		server.WithStore(ms),
		server.WithSchemaRegistry(reg),
		server.WithPasswordHasher(func(_ context.Context, p string) (string, error) {
			return "hashed:" + p, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	resp := doRequest(t, s, http.MethodPost, "/Users", `{"userName":"dave","password":"one"}`)
	var created map[string]interface{}
	decodeJSON(t, resp, &created)
	id, _ := created["id"].(string)

	stored := func() interface{} {
		res, err := ms.Get(context.Background(), "User", id)
		if err != nil {
			t.Fatal(err)
		}
		return res.Attributes["password"]
	}
	if got := stored(); got != "hashed:one" {
		t.Errorf("create: stored password %v, want hashed:one", got)
	}

	doRequest(t, s, http.MethodPut, "/Users/"+id, `{"userName":"dave","password":"two"}`)
	if got := stored(); got != "hashed:two" {
		t.Errorf("replace: stored password %v, want hashed:two", got)
	}

	for _, tc := range []struct{ ops, want string }{
		{`[{"op":"replace","path":"password","value":"three"}]`, "hashed:three"},
		{`[{"op":"replace","value":{"password":"four"}}]`, "hashed:four"},
	} {
		body, _ := json.Marshal(map[string]interface{}{
			"schemas":    []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
			"Operations": json.RawMessage(tc.ops),
		})
		resp := doRequest(t, s, http.MethodPatch, "/Users/"+id, string(body))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("patch: expected 200, got %d: %s", resp.StatusCode, readBody(t, resp))
		}
		if got := stored(); got != tc.want {
			t.Errorf("patch: stored password %v, want %s", got, tc.want)
		}
	}
}
