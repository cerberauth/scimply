// Package scimconnector_test contains integration tests for the SCIM connector.
//
// Unlike client_test.go (which uses hand-rolled httptest stubs), these tests
// start a real scimply server backed by a MemoryStore and exercise the full
// HTTP round-trip: connector → net/http → server → store → connector.
// This validates SCIM protocol fidelity end-to-end rather than mocked behavior.
package scimconnector_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	scimconnector "github.com/cerberauth/scimply/connector/scim"
	"github.com/cerberauth/scimply/resource"
	"github.com/cerberauth/scimply/schema"
	"github.com/cerberauth/scimply/server"
	"github.com/cerberauth/scimply/store"
)

// newIntegrationServer starts a real scimply HTTP server backed by a fresh
// MemoryStore and returns the httptest.Server and the backing MemoryStore.
func newIntegrationServer(t *testing.T, opts ...server.Option) (*httptest.Server, *store.MemoryStore) {
	t.Helper()

	reg := schema.NewRegistry()
	reg.RegisterDefaults()

	ms := store.NewMemoryStore()

	baseOpts := []server.Option{
		server.WithStore(ms),
		server.WithSchemaRegistry(reg),
	}
	baseOpts = append(baseOpts, opts...)

	srv, err := server.New(baseOpts...)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	hs := httptest.NewServer(srv)
	t.Cleanup(hs.Close)
	return hs, ms
}

// newIntegrationClient creates a SCIM connector client pointing at hs.
func newIntegrationClient(t *testing.T, hs *httptest.Server, opts ...scimconnector.Option) *scimconnector.Client {
	t.Helper()
	base := []scimconnector.Option{
		scimconnector.WithBaseURL(hs.URL),
		scimconnector.WithHTTPClient(hs.Client()),
	}
	base = append(base, opts...)
	c, err := scimconnector.New(base...)
	if err != nil {
		t.Fatalf("scimconnector.New: %v", err)
	}
	return c
}

// userResource builds a minimal User resource map.
func userResource(userName string, extra map[string]interface{}) *resource.Resource {
	m := map[string]interface{}{
		"schemas":  []interface{}{schema.UserSchemaURI},
		"userName": userName,
	}
	for k, v := range extra {
		m[k] = v
	}
	return resource.FromMap(m)
}

// groupResource builds a minimal Group resource map.
func groupResource(displayName string, extra map[string]interface{}) *resource.Resource {
	m := map[string]interface{}{
		"schemas":     []interface{}{schema.GroupSchemaURI},
		"displayName": displayName,
	}
	for k, v := range extra {
		m[k] = v
	}
	return resource.FromMap(m)
}

// getStr extracts a string attribute from a resource for assertions.
func getStr(t *testing.T, r *resource.Resource, key string) string {
	t.Helper()
	v, ok := r.Attributes[key]
	if !ok {
		t.Errorf("attribute %q not found in resource", key)
		return ""
	}
	s, ok := v.(string)
	if !ok {
		t.Errorf("attribute %q is %T, want string", key, v)
	}
	return s
}

func getBool(t *testing.T, r *resource.Resource, key string) bool {
	t.Helper()
	v, ok := r.Attributes[key]
	if !ok {
		t.Errorf("attribute %q not found in resource", key)
		return false
	}
	b, ok := v.(bool)
	if !ok {
		t.Errorf("attribute %q is %T, want bool", key, v)
	}
	return b
}

func mustParseFilter(t *testing.T, expr string) resource.FilterExpression {
	t.Helper()
	f, err := resource.ParseFilter(expr)
	if err != nil {
		t.Fatalf("ParseFilter(%q): %v", expr, err)
	}
	return f
}

// itIsErr wraps errors.Is for integration test assertions.
// Note: isErr is defined in client_test.go; use this wrapper in this file.
func itIsErr(err, target error) bool {
	return errors.Is(err, target)
}

func TestIntegration_CreateGet_User(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	created, err := c.Create(ctx, "User", userResource("alice@example.com", nil))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected server-assigned ID, got empty")
	}
	if getStr(t, created, "userName") != "alice@example.com" {
		t.Errorf("userName mismatch after Create")
	}

	got, err := c.Get(ctx, "User", created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("Get returned ID %q, want %q", got.ID, created.ID)
	}
	if getStr(t, got, "userName") != "alice@example.com" {
		t.Errorf("userName mismatch after Get")
	}
}

func TestIntegration_CreateGet_Group(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	created, err := c.Create(ctx, "Group", groupResource("Engineering", nil))
	if err != nil {
		t.Fatalf("Create group: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected server-assigned ID")
	}
	if getStr(t, created, "displayName") != "Engineering" {
		t.Errorf("displayName mismatch after Create")
	}

	got, err := c.Get(ctx, "Group", created.ID)
	if err != nil {
		t.Fatalf("Get group: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("Get returned wrong ID")
	}
}

func TestIntegration_CreateList_AppearsInList(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	created, err := c.Create(ctx, "User", userResource("bob@example.com", nil))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	result, err := c.List(ctx, "User", store.ListParams{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.TotalResults < 1 {
		t.Fatalf("expected at least 1 result, got %d", result.TotalResults)
	}

	found := false
	for _, r := range result.Resources {
		if r.ID == created.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("created resource not found in List results")
	}
}

func TestIntegration_Replace(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	created, err := c.Create(ctx, "User", userResource("carol@example.com", nil))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated := userResource("carol@example.com", map[string]interface{}{
		"displayName": "Carol Doe",
		"active":      true,
	})

	replaced, err := c.Replace(ctx, "User", created.ID, updated)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if replaced.ID != created.ID {
		t.Errorf("Replace returned different ID")
	}
	if getStr(t, replaced, "displayName") != "Carol Doe" {
		t.Errorf("displayName not updated after Replace")
	}

	got, err := c.Get(ctx, "User", created.ID)
	if err != nil {
		t.Fatalf("Get after Replace: %v", err)
	}
	if getStr(t, got, "displayName") != "Carol Doe" {
		t.Errorf("displayName not persisted after Replace")
	}
	if !getBool(t, got, "active") {
		t.Errorf("active not persisted after Replace")
	}
}

func TestIntegration_Delete(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	created, err := c.Create(ctx, "User", userResource("dan@example.com", nil))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := c.Delete(ctx, "User", created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = c.Get(ctx, "User", created.ID)
	if !itIsErr(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound after Delete, got %v", err)
	}
}

func TestIntegration_DeleteNonExistent(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	err := c.Delete(ctx, "User", "nonexistent-id")
	if !itIsErr(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestIntegration_DoubleDelete(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	created, err := c.Create(ctx, "User", userResource("eve@example.com", nil))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := c.Delete(ctx, "User", created.ID); err != nil {
		t.Fatalf("first Delete: %v", err)
	}

	err = c.Delete(ctx, "User", created.ID)
	if !itIsErr(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound on second Delete, got %v", err)
	}
}

func TestIntegration_GetNotFound(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	_, err := c.Get(ctx, "User", "does-not-exist")
	if !itIsErr(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestIntegration_CreateDuplicateUserName(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	_, err := c.Create(ctx, "User", userResource("frank@example.com", nil))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = c.Create(ctx, "User", userResource("frank@example.com", nil))
	if !itIsErr(err, store.ErrConflict) {
		t.Errorf("expected ErrConflict on duplicate userName, got %v", err)
	}
}

func TestIntegration_ReplaceNotFound(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	_, err := c.Replace(ctx, "User", "ghost-id", userResource("ghost@example.com", nil))
	if !itIsErr(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestIntegration_PatchNotFound(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	ops := []resource.PatchOp{
		{Op: resource.PatchOpReplace, Value: map[string]interface{}{"displayName": "X"}},
	}
	_, err := c.Patch(ctx, "User", "ghost-id", ops)
	if !itIsErr(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestIntegration_PatchAdd_SimpleAttribute(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	created, err := c.Create(ctx, "User", userResource("grace@example.com", nil))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ops := []resource.PatchOp{
		{
			Op:    resource.PatchOpAdd,
			Value: map[string]interface{}{"displayName": "Grace Hopper"},
		},
	}
	patched, err := c.Patch(ctx, "User", created.ID, ops)
	if err != nil {
		t.Fatalf("Patch add: %v", err)
	}
	if getStr(t, patched, "displayName") != "Grace Hopper" {
		t.Errorf("displayName not set after Patch add")
	}
}

func TestIntegration_PatchReplace_SimpleAttribute(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	created, err := c.Create(ctx, "User", userResource("hank@example.com", map[string]interface{}{
		"displayName": "Hank Old",
	}))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ops := []resource.PatchOp{
		{
			Op:    resource.PatchOpReplace,
			Value: map[string]interface{}{"displayName": "Hank New"},
		},
	}
	patched, err := c.Patch(ctx, "User", created.ID, ops)
	if err != nil {
		t.Fatalf("Patch replace: %v", err)
	}
	if getStr(t, patched, "displayName") != "Hank New" {
		t.Errorf("displayName not replaced: got %q", getStr(t, patched, "displayName"))
	}
}

func TestIntegration_PatchRemove_SimpleAttribute(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	created, err := c.Create(ctx, "User", userResource("ivy@example.com", map[string]interface{}{
		"nickName": "Ivy",
	}))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	path, err := resource.ParsePatchPath("nickName")
	if err != nil {
		t.Fatalf("ParsePatchPath: %v", err)
	}
	ops := []resource.PatchOp{
		{Op: resource.PatchOpRemove, Path: path},
	}
	patched, err := c.Patch(ctx, "User", created.ID, ops)
	if err != nil {
		t.Fatalf("Patch remove: %v", err)
	}
	if _, ok := patched.Attributes["nickName"]; ok {
		t.Error("nickName should have been removed")
	}
}

func TestIntegration_PatchAdd_MultiValuedAttribute(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	created, err := c.Create(ctx, "User", userResource("jack@example.com", nil))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ops := []resource.PatchOp{
		{
			Op: resource.PatchOpAdd,
			Value: map[string]interface{}{
				"emails": []interface{}{
					map[string]interface{}{
						"value":   "jack@work.com",
						"type":    "work",
						"primary": true,
					},
				},
			},
		},
	}
	patched, err := c.Patch(ctx, "User", created.ID, ops)
	if err != nil {
		t.Fatalf("Patch add emails: %v", err)
	}

	emails, ok := patched.Attributes["emails"]
	if !ok {
		t.Fatal("emails attribute missing after Patch add")
	}
	list, ok := emails.([]interface{})
	if !ok {
		t.Fatalf("emails is %T, want []interface{}", emails)
	}
	if len(list) == 0 {
		t.Error("emails list is empty after Patch add")
	}
}

func TestIntegration_PatchReplace_WithPath(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	created, err := c.Create(ctx, "User", userResource("karen@example.com", map[string]interface{}{
		"displayName": "Karen Old",
	}))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	path, err := resource.ParsePatchPath("displayName")
	if err != nil {
		t.Fatalf("ParsePatchPath: %v", err)
	}
	ops := []resource.PatchOp{
		{Op: resource.PatchOpReplace, Path: path, Value: "Karen New"},
	}
	patched, err := c.Patch(ctx, "User", created.ID, ops)
	if err != nil {
		t.Fatalf("Patch replace with path: %v", err)
	}
	if getStr(t, patched, "displayName") != "Karen New" {
		t.Errorf("displayName not replaced via path: got %q", getStr(t, patched, "displayName"))
	}
}

func TestIntegration_Patch_MultipleOps(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	created, err := c.Create(ctx, "User", userResource("leo@example.com", map[string]interface{}{
		"nickName": "Leo",
	}))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	path, err := resource.ParsePatchPath("nickName")
	if err != nil {
		t.Fatalf("ParsePatchPath: %v", err)
	}
	ops := []resource.PatchOp{
		{Op: resource.PatchOpAdd, Value: map[string]interface{}{"displayName": "Leo Lion"}},
		{Op: resource.PatchOpReplace, Path: path, Value: "Lion"},
	}
	patched, err := c.Patch(ctx, "User", created.ID, ops)
	if err != nil {
		t.Fatalf("Patch multiple ops: %v", err)
	}
	if getStr(t, patched, "displayName") != "Leo Lion" {
		t.Errorf("displayName not set")
	}
	if getStr(t, patched, "nickName") != "Lion" {
		t.Errorf("nickName not replaced")
	}
}

func TestIntegration_PatchActivate(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	created, err := c.Create(ctx, "User", userResource("mia@example.com", map[string]interface{}{
		"active": false,
	}))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	path, err := resource.ParsePatchPath("active")
	if err != nil {
		t.Fatalf("ParsePatchPath: %v", err)
	}
	ops := []resource.PatchOp{
		{Op: resource.PatchOpReplace, Path: path, Value: true},
	}
	patched, err := c.Patch(ctx, "User", created.ID, ops)
	if err != nil {
		t.Fatalf("Patch activate: %v", err)
	}
	if !getBool(t, patched, "active") {
		t.Error("active should be true after Patch")
	}
}

func TestIntegration_Group_AddMember(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	user, err := c.Create(ctx, "User", userResource("nina@example.com", nil))
	if err != nil {
		t.Fatalf("Create user: %v", err)
	}
	group, err := c.Create(ctx, "Group", groupResource("Alpha", nil))
	if err != nil {
		t.Fatalf("Create group: %v", err)
	}

	ops := []resource.PatchOp{
		{
			Op: resource.PatchOpAdd,
			Value: map[string]interface{}{
				"members": []interface{}{
					map[string]interface{}{"value": user.ID},
				},
			},
		},
	}
	patched, err := c.Patch(ctx, "Group", group.ID, ops)
	if err != nil {
		t.Fatalf("Patch group add member: %v", err)
	}

	members, ok := patched.Attributes["members"]
	if !ok {
		t.Fatal("members attribute missing after Patch")
	}
	list, ok := members.([]interface{})
	if !ok || len(list) == 0 {
		t.Error("members list empty after Patch add")
	}
}

func TestIntegration_Group_RemoveMember(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	user, err := c.Create(ctx, "User", userResource("oscar@example.com", nil))
	if err != nil {
		t.Fatalf("Create user: %v", err)
	}
	group, err := c.Create(ctx, "Group", groupResource("Beta", map[string]interface{}{
		"members": []interface{}{
			map[string]interface{}{"value": user.ID},
		},
	}))
	if err != nil {
		t.Fatalf("Create group with member: %v", err)
	}

	// Remove the member via value filter path
	filterExpr := fmt.Sprintf(`members[value eq "%s"]`, user.ID)
	path, err := resource.ParsePatchPath(filterExpr)
	if err != nil {
		t.Fatalf("ParsePatchPath: %v", err)
	}
	ops := []resource.PatchOp{
		{Op: resource.PatchOpRemove, Path: path},
	}
	patched, err := c.Patch(ctx, "Group", group.ID, ops)
	if err != nil {
		t.Fatalf("Patch group remove member: %v", err)
	}

	members, ok := patched.Attributes["members"]
	if ok {
		list, _ := members.([]interface{})
		if len(list) != 0 {
			t.Errorf("expected empty members after remove, got %d", len(list))
		}
	}
}

func TestIntegration_Group_ReplaceMembers(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	u1, _ := c.Create(ctx, "User", userResource("u1@example.com", nil))
	u2, _ := c.Create(ctx, "User", userResource("u2@example.com", nil))
	group, err := c.Create(ctx, "Group", groupResource("Gamma", map[string]interface{}{
		"members": []interface{}{
			map[string]interface{}{"value": u1.ID},
		},
	}))
	if err != nil {
		t.Fatalf("Create group: %v", err)
	}

	// Replace members entirely
	ops := []resource.PatchOp{
		{
			Op: resource.PatchOpReplace,
			Value: map[string]interface{}{
				"members": []interface{}{
					map[string]interface{}{"value": u2.ID},
				},
			},
		},
	}
	patched, err := c.Patch(ctx, "Group", group.ID, ops)
	if err != nil {
		t.Fatalf("Patch replace members: %v", err)
	}

	members, ok := patched.Attributes["members"]
	if !ok {
		t.Fatal("members missing after replace")
	}
	list, _ := members.([]interface{})
	if len(list) != 1 {
		t.Errorf("expected 1 member after replace, got %d", len(list))
	}
}

func TestIntegration_Filter_EqUserName(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	if _, err := c.Create(ctx, "User", userResource("peter@example.com", nil)); err != nil {
		t.Fatalf("Create peter: %v", err)
	}
	if _, err := c.Create(ctx, "User", userResource("quinn@example.com", nil)); err != nil {
		t.Fatalf("Create quinn: %v", err)
	}

	f := mustParseFilter(t, `userName eq "peter@example.com"`)
	result, err := c.List(ctx, "User", store.ListParams{Filter: f})
	if err != nil {
		t.Fatalf("List with eq filter: %v", err)
	}
	if result.TotalResults != 1 {
		t.Errorf("expected 1 result, got %d", result.TotalResults)
	}
	if len(result.Resources) != 1 || getStr(t, result.Resources[0], "userName") != "peter@example.com" {
		t.Error("filter returned wrong user")
	}
}

func TestIntegration_Filter_CoUserName(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	if _, err := c.Create(ctx, "User", userResource("rose.smith@example.com", nil)); err != nil {
		t.Fatalf("Create rose: %v", err)
	}
	if _, err := c.Create(ctx, "User", userResource("other@example.com", nil)); err != nil {
		t.Fatalf("Create other: %v", err)
	}

	f := mustParseFilter(t, `userName co "rose"`)
	result, err := c.List(ctx, "User", store.ListParams{Filter: f})
	if err != nil {
		t.Fatalf("List with co filter: %v", err)
	}
	if result.TotalResults != 1 {
		t.Errorf("expected 1 result for co filter, got %d", result.TotalResults)
	}
}

func TestIntegration_Filter_SwUserName(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	c.Create(ctx, "User", userResource("sam@example.com", nil))
	c.Create(ctx, "User", userResource("notmatch@example.com", nil))

	f := mustParseFilter(t, `userName sw "sam"`)
	result, err := c.List(ctx, "User", store.ListParams{Filter: f})
	if err != nil {
		t.Fatalf("List with sw filter: %v", err)
	}
	if result.TotalResults != 1 {
		t.Errorf("expected 1 result for sw filter, got %d", result.TotalResults)
	}
}

func TestIntegration_Filter_EwUserName(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	c.Create(ctx, "User", userResource("ted@example.com", nil))
	c.Create(ctx, "User", userResource("ted@other.org", nil))

	f := mustParseFilter(t, `userName ew "@example.com"`)
	result, err := c.List(ctx, "User", store.ListParams{Filter: f})
	if err != nil {
		t.Fatalf("List with ew filter: %v", err)
	}
	if result.TotalResults != 1 {
		t.Errorf("expected 1 result for ew filter, got %d", result.TotalResults)
	}
}

func TestIntegration_Filter_BooleanActive(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	c.Create(ctx, "User", userResource("uma@example.com", map[string]interface{}{"active": true}))
	c.Create(ctx, "User", userResource("vince@example.com", map[string]interface{}{"active": false}))

	f := mustParseFilter(t, `active eq true`)
	result, err := c.List(ctx, "User", store.ListParams{Filter: f})
	if err != nil {
		t.Fatalf("List with boolean filter: %v", err)
	}
	if result.TotalResults != 1 {
		t.Errorf("expected 1 active user, got %d", result.TotalResults)
	}
	if getStr(t, result.Resources[0], "userName") != "uma@example.com" {
		t.Error("wrong user returned for active eq true filter")
	}
}

func TestIntegration_Filter_PrOperator(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	c.Create(ctx, "User", userResource("wendy@example.com", map[string]interface{}{
		"displayName": "Wendy",
	}))
	c.Create(ctx, "User", userResource("xavier@example.com", nil))

	f := mustParseFilter(t, `displayName pr`)
	result, err := c.List(ctx, "User", store.ListParams{Filter: f})
	if err != nil {
		t.Fatalf("List with pr filter: %v", err)
	}
	if result.TotalResults != 1 {
		t.Errorf("expected 1 result for pr filter, got %d", result.TotalResults)
	}
}

func TestIntegration_Filter_NotExpression(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	c.Create(ctx, "User", userResource("yara@example.com", nil))
	c.Create(ctx, "User", userResource("zach@example.com", nil))

	f := mustParseFilter(t, `not (userName eq "yara@example.com")`)
	result, err := c.List(ctx, "User", store.ListParams{Filter: f})
	if err != nil {
		t.Fatalf("List with not filter: %v", err)
	}
	if result.TotalResults != 1 {
		t.Errorf("expected 1 result for not filter, got %d", result.TotalResults)
	}
	if getStr(t, result.Resources[0], "userName") != "zach@example.com" {
		t.Error("not filter returned wrong user")
	}
}

func TestIntegration_Filter_LogicalAnd(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	c.Create(ctx, "User", userResource("ada@example.com", map[string]interface{}{"active": true}))
	c.Create(ctx, "User", userResource("ada@example.com2", map[string]interface{}{"active": false}))
	// Note: second create will conflict on userName unless we use a different userName
	c.Create(ctx, "User", userResource("ben@example.com", map[string]interface{}{"active": true}))

	f := mustParseFilter(t, `userName eq "ada@example.com" and active eq true`)
	result, err := c.List(ctx, "User", store.ListParams{Filter: f})
	if err != nil {
		t.Fatalf("List with and filter: %v", err)
	}
	if result.TotalResults != 1 {
		t.Errorf("expected 1 result for and filter, got %d", result.TotalResults)
	}
}

func TestIntegration_Filter_LogicalOr(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	c.Create(ctx, "User", userResource("cat@example.com", nil))
	c.Create(ctx, "User", userResource("dog@example.com", nil))
	c.Create(ctx, "User", userResource("elk@example.com", nil))

	f := mustParseFilter(t, `userName eq "cat@example.com" or userName eq "dog@example.com"`)
	result, err := c.List(ctx, "User", store.ListParams{Filter: f})
	if err != nil {
		t.Fatalf("List with or filter: %v", err)
	}
	if result.TotalResults != 2 {
		t.Errorf("expected 2 results for or filter, got %d", result.TotalResults)
	}
}

func TestIntegration_Filter_NoFilter_ReturnsAll(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		c.Create(ctx, "User", userResource(fmt.Sprintf("user%d@example.com", i), nil))
	}

	result, err := c.List(ctx, "User", store.ListParams{})
	if err != nil {
		t.Fatalf("List without filter: %v", err)
	}
	if result.TotalResults != 3 {
		t.Errorf("expected 3 results, got %d", result.TotalResults)
	}
}

func TestIntegration_Pagination_StartIndexCount(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := c.Create(ctx, "User", userResource(fmt.Sprintf("page%d@example.com", i), nil))
		if err != nil {
			t.Fatalf("Create user %d: %v", i, err)
		}
	}

	result, err := c.List(ctx, "User", store.ListParams{StartIndex: 1, Count: 2})
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if result.TotalResults != 5 {
		t.Errorf("TotalResults = %d, want 5", result.TotalResults)
	}
	if len(result.Resources) != 2 {
		t.Errorf("page size = %d, want 2", len(result.Resources))
	}
	if result.ItemsPerPage != 2 {
		t.Errorf("ItemsPerPage = %d, want 2", result.ItemsPerPage)
	}
	if result.StartIndex != 1 {
		t.Errorf("StartIndex = %d, want 1", result.StartIndex)
	}
}

func TestIntegration_Pagination_SecondPage(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		c.Create(ctx, "User", userResource(fmt.Sprintf("pg2user%d@example.com", i), nil))
	}

	result, err := c.List(ctx, "User", store.ListParams{StartIndex: 3, Count: 2})
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if result.TotalResults != 5 {
		t.Errorf("TotalResults = %d, want 5", result.TotalResults)
	}
	if len(result.Resources) != 2 {
		t.Errorf("page 2 size = %d, want 2", len(result.Resources))
	}
	if result.StartIndex != 3 {
		t.Errorf("StartIndex = %d, want 3", result.StartIndex)
	}
}

func TestIntegration_Pagination_BeyondTotal(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	c.Create(ctx, "User", userResource("only@example.com", nil))

	result, err := c.List(ctx, "User", store.ListParams{StartIndex: 100, Count: 10})
	if err != nil {
		t.Fatalf("List beyond total: %v", err)
	}
	if result.TotalResults != 1 {
		t.Errorf("TotalResults = %d, want 1", result.TotalResults)
	}
	if len(result.Resources) != 0 {
		t.Errorf("expected 0 resources for out-of-range page, got %d", len(result.Resources))
	}
}

func TestIntegration_Pagination_SingleItemPages(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	names := []string{"aa@ex.com", "bb@ex.com", "cc@ex.com"}
	for _, n := range names {
		c.Create(ctx, "User", userResource(n, nil))
	}

	seen := map[string]bool{}
	for page := 1; page <= 3; page++ {
		result, err := c.List(ctx, "User", store.ListParams{
			StartIndex: page,
			Count:      1,
			SortBy:     "userName",
			SortOrder:  store.SortAscending,
		})
		if err != nil {
			t.Fatalf("List page %d: %v", page, err)
		}
		if len(result.Resources) != 1 {
			t.Errorf("page %d: expected 1 resource, got %d", page, len(result.Resources))
			continue
		}
		id := result.Resources[0].ID
		if seen[id] {
			t.Errorf("duplicate resource %q across pages", id)
		}
		seen[id] = true
	}
	if len(seen) != 3 {
		t.Errorf("expected 3 unique resources across 3 pages, got %d", len(seen))
	}
}

func TestIntegration_Sort_Ascending(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	// Create users with predictable displayNames for sorting.
	users := []string{"Charlie", "Alice", "Bob"}
	for _, name := range users {
		c.Create(ctx, "User", userResource(
			fmt.Sprintf("%s@example.com", name),
			map[string]interface{}{"displayName": name},
		))
	}

	result, err := c.List(ctx, "User", store.ListParams{
		SortBy:    "displayName",
		SortOrder: store.SortAscending,
	})
	if err != nil {
		t.Fatalf("List sorted asc: %v", err)
	}
	if len(result.Resources) < 3 {
		t.Fatalf("expected 3 resources, got %d", len(result.Resources))
	}

	names := []string{
		getStr(t, result.Resources[0], "displayName"),
		getStr(t, result.Resources[1], "displayName"),
		getStr(t, result.Resources[2], "displayName"),
	}
	if names[0] > names[1] || names[1] > names[2] {
		t.Errorf("expected ascending order, got %v", names)
	}
}

func TestIntegration_Sort_Descending(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	users := []string{"Zebra", "Mango", "Apple"}
	for _, name := range users {
		c.Create(ctx, "User", userResource(
			fmt.Sprintf("%s@sort.com", name),
			map[string]interface{}{"displayName": name},
		))
	}

	result, err := c.List(ctx, "User", store.ListParams{
		SortBy:    "displayName",
		SortOrder: store.SortDescending,
	})
	if err != nil {
		t.Fatalf("List sorted desc: %v", err)
	}
	if len(result.Resources) < 3 {
		t.Fatalf("expected 3 resources, got %d", len(result.Resources))
	}

	names := []string{
		getStr(t, result.Resources[0], "displayName"),
		getStr(t, result.Resources[1], "displayName"),
		getStr(t, result.Resources[2], "displayName"),
	}
	if names[0] < names[1] || names[1] < names[2] {
		t.Errorf("expected descending order, got %v", names)
	}
}

func TestIntegration_Auth_BearerToken_Accepted(t *testing.T) {
	const token = "supersecret"

	hs, _ := newIntegrationServer(t,
		server.WithBearerTokenAuth(func(tok string) (bool, error) {
			return tok == token, nil
		}),
	)

	c := newIntegrationClient(t, hs, scimconnector.WithBearerToken(token))
	ctx := context.Background()

	_, err := c.Create(ctx, "User", userResource("auth@example.com", nil))
	if err != nil {
		t.Errorf("Create with valid token: %v", err)
	}
}

func TestIntegration_Auth_BearerToken_Rejected(t *testing.T) {
	hs, _ := newIntegrationServer(t,
		server.WithBearerTokenAuth(func(tok string) (bool, error) {
			return tok == "correct-token", nil
		}),
	)

	c := newIntegrationClient(t, hs, scimconnector.WithBearerToken("wrong-token"))
	ctx := context.Background()

	_, err := c.Create(ctx, "User", userResource("unauthorized@example.com", nil))
	if err == nil {
		t.Error("expected error with wrong token, got nil")
	}
}

func TestIntegration_Auth_NoToken_Rejected(t *testing.T) {
	hs, _ := newIntegrationServer(t,
		server.WithBearerTokenAuth(func(tok string) (bool, error) {
			return false, nil
		}),
	)

	c := newIntegrationClient(t, hs) // no auth option
	ctx := context.Background()

	_, err := c.Get(ctx, "User", "any-id")
	if err == nil {
		t.Error("expected error without token, got nil")
	}
}

func TestIntegration_Auth_BasicAuth_Accepted(t *testing.T) {
	// The server uses a custom AuthFunc to validate Basic auth.
	hs, _ := newIntegrationServer(t,
		server.WithAuthFunc(func(r *http.Request) (bool, error) {
			user, pass, ok := r.BasicAuth()
			return ok && user == "admin" && pass == "pass", nil
		}),
	)

	c := newIntegrationClient(t, hs, scimconnector.WithBasicAuth("admin", "pass"))
	ctx := context.Background()

	_, err := c.Create(ctx, "User", userResource("basicauth@example.com", nil))
	if err != nil {
		t.Errorf("Create with Basic auth: %v", err)
	}
}

func TestIntegration_TrailingSlashBaseURL(t *testing.T) {
	hs, _ := newIntegrationServer(t)

	// Add trailing slash — constructor should normalise it.
	c, err := scimconnector.New(
		scimconnector.WithBaseURL(hs.URL+"/"),
		scimconnector.WithHTTPClient(hs.Client()),
	)
	if err != nil {
		t.Fatalf("New with trailing slash: %v", err)
	}

	_, err = c.Create(context.Background(), "User", userResource("trailing@example.com", nil))
	if err != nil {
		t.Errorf("Create after trailing slash normalisation: %v", err)
	}
}

func TestIntegration_IDWithSpecialChars(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	// Create a user, then verify the GET URL-encodes the assigned ID
	// (IDs from MemoryStore are hex, but we verify the round-trip works).
	created, err := c.Create(ctx, "User", userResource("spec@example.com", nil))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Re-fetch by ID to ensure URL path encoding is transparent.
	got, err := c.Get(ctx, "User", created.ID)
	if err != nil {
		t.Fatalf("Get with encoded ID: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, created.ID)
	}
}

func TestIntegration_New_MissingBaseURL(t *testing.T) {
	_, err := scimconnector.New()
	if err == nil {
		t.Error("expected error when BaseURL is missing, got nil")
	}
}

// newStandaloneClient creates a SCIM connector client for a raw httptest.Server
// that is NOT backed by a scimply server (e.g. a hand-written stub for retry tests).
func newStandaloneClient(t *testing.T, srv *httptest.Server) *scimconnector.Client {
	t.Helper()
	c, err := scimconnector.New(
		scimconnector.WithBaseURL(srv.URL),
		scimconnector.WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("scimconnector.New: %v", err)
	}
	return c
}

func TestIntegration_Retry_429ThenSuccess(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeJSON(w, http.StatusCreated, sampleUserMap("retry-id"))
	}))
	defer srv.Close()

	c := newStandaloneClient(t, srv)

	created, err := c.Create(context.Background(), "User", userResource("retry@example.com", nil))
	if err != nil {
		t.Fatalf("Create after 429 retry: %v", err)
	}
	if created.ID != "retry-id" {
		t.Errorf("got ID %q, want %q", created.ID, "retry-id")
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestIntegration_Retry_ExhaustedRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newStandaloneClient(t, srv)

	_, err := c.Create(context.Background(), "User", userResource("ratelimited@example.com", nil))
	if err == nil {
		t.Error("expected error after exhausted retries, got nil")
	}
}

func TestIntegration_Retry_ContextCancelledDuringWait(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return 429 with a long Retry-After so the context will cancel first.
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newStandaloneClient(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := c.Create(ctx, "User", userResource("cancelled@example.com", nil))
	if err == nil {
		t.Error("expected error on cancelled context, got nil")
	}
}

func TestIntegration_ContentTypeHeader(t *testing.T) {
	var gotContentType string
	var gotAccept string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			gotContentType = r.Header.Get("Content-Type")
			gotAccept = r.Header.Get("Accept")
		}
		writeJSON(w, http.StatusCreated, sampleUserMap("ct-id"))
	}))
	defer srv.Close()

	c := newStandaloneClient(t, srv)
	c.Create(context.Background(), "User", userResource("ct@example.com", nil))

	if gotContentType != "application/scim+json" {
		t.Errorf("Content-Type = %q, want application/scim+json", gotContentType)
	}
	if gotAccept != "application/scim+json" {
		t.Errorf("Accept = %q, want application/scim+json", gotAccept)
	}
}

func TestIntegration_FullLifecycle_User(t *testing.T) {
	hs, _ := newIntegrationServer(t)
	c := newIntegrationClient(t, hs)
	ctx := context.Background()

	created, err := c.Create(ctx, "User", userResource("lifecycle@example.com", map[string]interface{}{
		"displayName": "Initial Name",
		"active":      true,
	}))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := created.ID

	got, err := c.Get(ctx, "User", id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if getStr(t, got, "displayName") != "Initial Name" {
		t.Error("displayName mismatch after Get")
	}

	f := mustParseFilter(t, `userName eq "lifecycle@example.com"`)
	list, err := c.List(ctx, "User", store.ListParams{Filter: f})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.TotalResults != 1 {
		t.Errorf("List TotalResults = %d, want 1", list.TotalResults)
	}

	replaced, err := c.Replace(ctx, "User", id, userResource("lifecycle@example.com", map[string]interface{}{
		"displayName": "Updated Name",
		"active":      false,
	}))
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if getStr(t, replaced, "displayName") != "Updated Name" {
		t.Error("displayName not updated after Replace")
	}

	path, _ := resource.ParsePatchPath("active")
	ops := []resource.PatchOp{{Op: resource.PatchOpReplace, Path: path, Value: true}}
	patched, err := c.Patch(ctx, "User", id, ops)
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if !getBool(t, patched, "active") {
		t.Error("active should be true after Patch re-activate")
	}

	if err := c.Delete(ctx, "User", id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = c.Get(ctx, "User", id)
	if !itIsErr(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound after Delete, got %v", err)
	}
}
