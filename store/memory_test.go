package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/cerberauth/scimply/resource"
)

func newUser(userName string) *resource.Resource {
	return &resource.Resource{
		Schemas: []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		Attributes: map[string]interface{}{
			"userName": userName,
		},
	}
}

func TestMemoryStore_Create(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	r, err := s.Create(ctx, "User", newUser("bjensen"))
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}
	if r.ID == "" {
		t.Error("Create: ID should be set")
	}
	if r.Meta.ResourceType != "User" {
		t.Errorf("Create: Meta.ResourceType = %q, want User", r.Meta.ResourceType)
	}
	if r.Meta.Created.IsZero() {
		t.Error("Create: Meta.Created should be set")
	}
	if r.Meta.LastModified.IsZero() {
		t.Error("Create: Meta.LastModified should be set")
	}
}

func TestMemoryStore_Create_DuplicateUserName(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	_, err := s.Create(ctx, "User", newUser("bjensen"))
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err = s.Create(ctx, "User", newUser("bjensen"))
	if !errors.Is(err, ErrConflict) {
		t.Errorf("duplicate userName: got %v, want ErrConflict", err)
	}
}

func TestMemoryStore_Get(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	created, _ := s.Create(ctx, "User", newUser("jsmith"))

	got, err := s.Get(ctx, "User", created.ID)
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("Get: ID = %q, want %q", got.ID, created.ID)
	}
}

func TestMemoryStore_Get_NotFound(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	_, err := s.Get(ctx, "User", "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing: got %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_List(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	for _, u := range []string{"alice", "bob", "charlie"} {
		if _, err := s.Create(ctx, "User", newUser(u)); err != nil {
			t.Fatalf("Create %s: %v", u, err)
		}
	}

	result, err := s.List(ctx, "User", ListParams{StartIndex: 1, Count: -1})
	if err != nil {
		t.Fatalf("List: unexpected error: %v", err)
	}
	if result.TotalResults != 3 {
		t.Errorf("TotalResults = %d, want 3", result.TotalResults)
	}
	if len(result.Resources) != 3 {
		t.Errorf("len(Resources) = %d, want 3", len(result.Resources))
	}
}

func TestMemoryStore_List_Pagination(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := s.Create(ctx, "User", newUser("user"+string(rune('a'+i)))); err != nil {
			t.Fatalf("Create user %d: %v", i, err)
		}
	}

	result, err := s.List(ctx, "User", ListParams{StartIndex: 2, Count: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.TotalResults != 5 {
		t.Errorf("TotalResults = %d, want 5", result.TotalResults)
	}
	if result.ItemsPerPage != 2 {
		t.Errorf("ItemsPerPage = %d, want 2", result.ItemsPerPage)
	}
	if result.StartIndex != 2 {
		t.Errorf("StartIndex = %d, want 2", result.StartIndex)
	}
}

func TestMemoryStore_List_WithFilter(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	if _, err := s.Create(ctx, "User", newUser("alice")); err != nil {
		t.Fatalf("Create alice: %v", err)
	}
	if _, err := s.Create(ctx, "User", newUser("bob")); err != nil {
		t.Fatalf("Create bob: %v", err)
	}
	if _, err := s.Create(ctx, "User", newUser("charlie")); err != nil {
		t.Fatalf("Create charlie: %v", err)
	}

	filter, err := resource.ParseFilter(`userName eq "alice"`)
	if err != nil {
		t.Fatalf("ParseFilter: %v", err)
	}

	result, err := s.List(ctx, "User", ListParams{Filter: filter, StartIndex: 1, Count: -1})
	if err != nil {
		t.Fatalf("List with filter: %v", err)
	}
	if result.TotalResults != 1 {
		t.Errorf("TotalResults = %d, want 1", result.TotalResults)
	}
	if len(result.Resources) != 1 {
		t.Errorf("len(Resources) = %d, want 1", len(result.Resources))
	}
}

func TestMemoryStore_Replace(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	created, _ := s.Create(ctx, "User", newUser("jsmith"))

	updated := &resource.Resource{
		Schemas: created.Schemas,
		Attributes: map[string]interface{}{
			"userName":    "jsmith",
			"displayName": "John Smith",
		},
	}

	replaced, err := s.Replace(ctx, "User", created.ID, updated)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if replaced.ID != created.ID {
		t.Errorf("Replace: ID changed from %q to %q", created.ID, replaced.ID)
	}
	if !replaced.Meta.Created.Equal(created.Meta.Created) {
		t.Error("Replace: Meta.Created should be preserved")
	}
	if replaced.Meta.LastModified.Before(created.Meta.LastModified) {
		t.Error("Replace: Meta.LastModified should not be before Created.LastModified")
	}
	if dn, _ := replaced.Attributes["displayName"].(string); dn != "John Smith" {
		t.Errorf("Replace: displayName = %q, want John Smith", dn)
	}
}

func TestMemoryStore_Replace_NotFound(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	_, err := s.Replace(ctx, "User", "nonexistent", newUser("x"))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Replace missing: got %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_Patch(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	created, _ := s.Create(ctx, "User", newUser("pjones"))

	ops := []resource.PatchOp{
		{
			Op:    resource.PatchOpReplace,
			Path:  nil,
			Value: map[string]interface{}{"displayName": "Peter Jones"},
		},
	}

	patched, err := s.Patch(ctx, "User", created.ID, ops)
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if patched.ID != created.ID {
		t.Errorf("Patch: ID changed")
	}
	if dn, _ := patched.Attributes["displayName"].(string); dn != "Peter Jones" {
		t.Errorf("Patch: displayName = %q, want Peter Jones", dn)
	}
}

func TestMemoryStore_Patch_NotFound(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	_, err := s.Patch(ctx, "User", "nonexistent", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Patch missing: got %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	created, _ := s.Create(ctx, "User", newUser("toDelete"))

	if err := s.Delete(ctx, "User", created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := s.Get(ctx, "User", created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: got %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_Delete_NotFound(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	err := s.Delete(ctx, "User", "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete missing: got %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_ContextCancellation(t *testing.T) {
	s := NewMemoryStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Create(ctx, "User", newUser("x"))
	if err == nil {
		t.Error("Create with cancelled context should return error")
	}

	_, err = s.Get(ctx, "User", "any")
	if err == nil {
		t.Error("Get with cancelled context should return error")
	}

	_, err = s.List(ctx, "User", ListParams{})
	if err == nil {
		t.Error("List with cancelled context should return error")
	}

	_, err = s.Replace(ctx, "User", "any", newUser("x"))
	if err == nil {
		t.Error("Replace with cancelled context should return error")
	}

	_, err = s.Patch(ctx, "User", "any", nil)
	if err == nil {
		t.Error("Patch with cancelled context should return error")
	}

	err = s.Delete(ctx, "User", "any")
	if err == nil {
		t.Error("Delete with cancelled context should return error")
	}
}

func TestMemoryStore_Concurrent(t *testing.T) {
	t.Parallel()

	s := NewMemoryStore()
	ctx := context.Background()

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			userName := "user" + string(rune('a'+i%26))

			s.Create(ctx, "User", newUser(userName))
		}(i)
	}
	wg.Wait()

	result, err := s.List(ctx, "User", ListParams{StartIndex: 1, Count: -1})
	if err != nil {
		t.Fatalf("List after concurrent creates: %v", err)
	}
	if result.TotalResults == 0 {
		t.Error("expected at least one resource after concurrent creates")
	}
}

func TestMemoryStore_List_EmptyType(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	result, err := s.List(ctx, "User", ListParams{StartIndex: 1})
	if err != nil {
		t.Fatalf("List on empty store: %v", err)
	}
	if result.TotalResults != 0 {
		t.Errorf("TotalResults = %d, want 0", result.TotalResults)
	}
	if len(result.Resources) != 0 {
		t.Errorf("Resources should be empty")
	}
}

func TestMemoryStore_WithHooks(t *testing.T) {
	var preCreateCalled, postCreateCalled bool

	hooks := Hooks{
		PreCreate: []PreCreateHook{
			func(ctx context.Context, rt string, r *resource.Resource) error {
				preCreateCalled = true
				return nil
			},
		},
		PostCreate: []PostCreateHook{
			func(ctx context.Context, rt string, r *resource.Resource) {
				postCreateCalled = true
			},
		},
	}

	s := NewMemoryStore().WithHooks(hooks)
	ctx := context.Background()

	_, err := s.Create(ctx, "User", newUser("hookUser"))
	if err != nil {
		t.Fatalf("Create with hooks: %v", err)
	}
	if !preCreateCalled {
		t.Error("PreCreate hook was not called")
	}
	if !postCreateCalled {
		t.Error("PostCreate hook was not called")
	}
}

func TestMemoryStore_PreCreateHook_Abort(t *testing.T) {
	hooks := Hooks{
		PreCreate: []PreCreateHook{
			func(ctx context.Context, rt string, r *resource.Resource) error {
				return ErrInternal
			},
		},
	}

	s := NewMemoryStore().WithHooks(hooks)
	ctx := context.Background()

	_, err := s.Create(ctx, "User", newUser("blocked"))
	if !errors.Is(err, ErrInternal) {
		t.Errorf("hook abort: got %v, want ErrInternal", err)
	}
}
