package mongodb_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	mongoconn "github.com/cerberauth/scimply/connector/mongodb"
	"github.com/cerberauth/scimply/resource"
	"github.com/cerberauth/scimply/store"
)

func openStore(t *testing.T) *mongoconn.Store {
	t.Helper()
	uri := os.Getenv("TEST_MONGODB_URI")
	if uri == "" {
		t.Skip("TEST_MONGODB_URI not set; skipping integration test")
	}

	s, err := mongoconn.New(
		mongoconn.WithURI(uri),
		mongoconn.WithDatabase("scimply_integration_test"),
		mongoconn.WithAutoMigrate(true),
		mongoconn.WithCollectionPrefix("scimtest_"),
		mongoconn.WithTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() {
		_ = s.Close(context.Background())
	})
	return s
}

func newUser(userName string) *resource.Resource {
	return &resource.Resource{
		Schemas: []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		Attributes: map[string]interface{}{
			"userName": userName,
			"active":   true,
		},
	}
}

func TestMongoDB_Healthy(t *testing.T) {
	s := openStore(t)
	if err := s.Healthy(context.Background()); err != nil {
		t.Fatalf("Healthy: %v", err)
	}
}

func TestMongoDB_CreateAndGet(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	created, err := s.Create(ctx, "user", newUser("alice-"+randomSuffix()+"@example.com"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if created.Meta.Created.IsZero() {
		t.Fatal("expected non-zero meta.created")
	}

	got, err := s.Get(ctx, "user", created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID mismatch: got %q want %q", got.ID, created.ID)
	}
}

func TestMongoDB_GetNotFound(t *testing.T) {
	s := openStore(t)
	_, err := s.Get(context.Background(), "user", "nonexistent-id-xyz")
	if !isNotFound(err) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMongoDB_CreateConflict(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	userName := "conflict-" + randomSuffix() + "@example.com"
	_, err := s.Create(ctx, "user", newUser(userName))
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err = s.Create(ctx, "user", newUser(userName))
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !isConflict(err) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestMongoDB_List(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	suffix := randomSuffix()
	for i := 0; i < 3; i++ {
		_, err := s.Create(ctx, "user", newUser(fmt.Sprintf("%s-%d@example.com", suffix, i)))
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	result, err := s.List(ctx, "user", store.ListParams{StartIndex: 1, Count: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.TotalResults < 3 {
		t.Errorf("expected at least 3 results, got %d", result.TotalResults)
	}
}

func TestMongoDB_Replace(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	created, err := s.Create(ctx, "user", newUser("replace-"+randomSuffix()+"@example.com"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated := created.Clone()
	updated.Attributes["displayName"] = "Alice"

	replaced, err := s.Replace(ctx, "user", created.ID, updated)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if replaced.Meta.LastModified.Before(replaced.Meta.Created) {
		t.Error("lastModified should not be before created")
	}
}

func TestMongoDB_Patch(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	created, err := s.Create(ctx, "user", newUser("patch-"+randomSuffix()+"@example.com"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ops := []resource.PatchOp{
		{
			Op: resource.PatchOpReplace,
			Value: map[string]interface{}{
				"displayName": "Patched User",
			},
		},
	}
	patched, err := s.Patch(ctx, "user", created.ID, ops)
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if patched.Attributes["displayName"] != "Patched User" {
		t.Errorf("expected displayName='Patched User', got %v", patched.Attributes["displayName"])
	}
}

func TestMongoDB_Delete(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	created, err := s.Create(ctx, "user", newUser("delete-"+randomSuffix()+"@example.com"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Delete(ctx, "user", created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = s.Get(ctx, "user", created.ID)
	if !isNotFound(err) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestMongoDB_DeleteNotFound(t *testing.T) {
	s := openStore(t)
	err := s.Delete(context.Background(), "user", "nonexistent-id-xyz")
	if !isNotFound(err) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}

func isConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "uniqueness constraint")
}

func randomSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
