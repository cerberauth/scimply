package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/cerberauth/scimply/resource"
	"github.com/cerberauth/scimply/store"
)

func setupPostgresStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping integration test")
	}

	s, err := New(
		WithDSN(dsn),
		WithAutoMigrate(true),
		WithTablePrefix("test_scim_"),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	table := s.tableName("User")
	ddl := `CREATE TABLE IF NOT EXISTS ` + quoteIdent(table) + ` (
		id            TEXT NOT NULL PRIMARY KEY,
		external_id   TEXT,
		user_name     TEXT UNIQUE,
		meta_created  TIMESTAMPTZ NOT NULL DEFAULT now(),
		meta_last_mod TIMESTAMPTZ NOT NULL DEFAULT now(),
		meta_version  TEXT,
		active        BOOLEAN NOT NULL DEFAULT true,
		data          JSONB NOT NULL,
		schemas       TEXT[]
	)`
	if _, err := s.pool.Exec(ctx, ddl); err != nil {
		t.Fatalf("create test table: %v", err)
	}

	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), `DROP TABLE IF EXISTS `+quoteIdent(table))
		_ = s.Close(context.Background())
	})

	return s
}

func newTestUser(userName string) *resource.Resource {
	return &resource.Resource{
		Schemas: []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		Attributes: map[string]interface{}{
			"userName": userName,
			"active":   true,
		},
	}
}

func TestPostgresCreate(t *testing.T) {
	s := setupPostgresStore(t)
	ctx := context.Background()

	res, err := s.Create(ctx, "User", newTestUser("alice@example.com"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.ID == "" {
		t.Error("expected non-empty ID after Create")
	}
	if res.Meta.Created.IsZero() {
		t.Error("expected non-zero meta.created after Create")
	}
}

func TestPostgresGet(t *testing.T) {
	s := setupPostgresStore(t)
	ctx := context.Background()

	created, err := s.Create(ctx, "User", newTestUser("bob@example.com"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get(ctx, "User", created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("Get ID mismatch: got %q, want %q", got.ID, created.ID)
	}
}

func TestPostgresList(t *testing.T) {
	s := setupPostgresStore(t)
	ctx := context.Background()

	for _, name := range []string{"list1@example.com", "list2@example.com"} {
		if _, err := s.Create(ctx, "User", newTestUser(name)); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}

	result, err := s.List(ctx, "User", store.ListParams{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.TotalResults < 2 {
		t.Errorf("expected at least 2 results, got %d", result.TotalResults)
	}
}

func TestPostgresReplace(t *testing.T) {
	s := setupPostgresStore(t)
	ctx := context.Background()

	created, err := s.Create(ctx, "User", newTestUser("replace@example.com"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated := newTestUser("replace-updated@example.com")
	replaced, err := s.Replace(ctx, "User", created.ID, updated)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if replaced.ID != created.ID {
		t.Errorf("Replace changed ID: got %q, want %q", replaced.ID, created.ID)
	}
	if replaced.Meta.Created != created.Meta.Created {
		t.Errorf("Replace changed meta.created")
	}
}

func TestPostgresPatch(t *testing.T) {
	s := setupPostgresStore(t)
	ctx := context.Background()

	created, err := s.Create(ctx, "User", newTestUser("patch@example.com"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ops := []resource.PatchOp{
		{
			Op:    resource.PatchOpReplace,
			Value: map[string]interface{}{"active": false},
		},
	}
	patched, err := s.Patch(ctx, "User", created.ID, ops)
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if active, ok := patched.Attributes["active"].(bool); !ok || active {
		t.Errorf("expected active=false after Patch, got %v", patched.Attributes["active"])
	}
}

func TestPostgresDelete(t *testing.T) {
	s := setupPostgresStore(t)
	ctx := context.Background()

	created, err := s.Create(ctx, "User", newTestUser("delete@example.com"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Delete(ctx, "User", created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = s.Get(ctx, "User", created.ID)
	if err == nil {
		t.Error("expected ErrNotFound after Delete, got nil")
	}
}
