package main

import (
	"context"
	"log"
	"net/http"
	"os"

	sqlconn "github.com/cerberauth/scimply/connector/sql"

	"github.com/cerberauth/scimply/connector/postgres"
	"github.com/cerberauth/scimply/schema"
	"github.com/cerberauth/scimply/server"
)

// This example shows how to run a SCIM 2.0 server backed by an existing
// PostgreSQL schema. Rather than letting scimply create its own tables,
// we map each SCIM resource type to a specific table and declare which
// column holds each SCIM attribute.
//
// Expected schema (simplified):
//
//	CREATE TABLE accounts (
//	    id         UUID PRIMARY KEY,
//	    email      TEXT UNIQUE NOT NULL,
//	    is_active  BOOLEAN NOT NULL DEFAULT true,
//	    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
//	    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
//	);
//
//	CREATE TABLE profiles (
//	    account_id UUID PRIMARY KEY REFERENCES accounts(id),
//	    first_name TEXT,
//	    last_name  TEXT,
//	    display    TEXT
//	);
//
//	CREATE TABLE teams (
//	    id           UUID PRIMARY KEY,
//	    display_name TEXT NOT NULL,
//	    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
//	    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
//	);
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://scim:scim@localhost:5432/scim"
	}
	token := os.Getenv("SCIM_TOKEN")
	if token == "" {
		token = "dev-token"
	}

	reg := schema.NewRegistry()
	reg.RegisterDefaults()

	pgStore, err := postgres.New(
		postgres.WithDSN(dsn),

		// ── Map "User" to an existing "accounts" + "profiles" schema ──────────
		//
		// WithResourceConfig is the all-in-one option. Use WithTableName,
		// WithFieldMapping, and WithJoin individually for the same effect.
		postgres.WithResourceConfig("User", postgres.ResourceTableConfig{
			Table: "accounts",
			FieldMappings: map[string]sqlconn.ColumnRef{
				// Required: primary key
				"id": {Column: "id"},
				// SCIM "userName" lives in accounts.email
				"userName": {Column: "email"},
				// Lifecycle flags
				"active":            {Column: "is_active"},
				"meta.created":      {Column: "created_at"},
				"meta.lastModified": {Column: "updated_at"},
				// Name sub-attributes live in the joined "profiles" table
				"name.givenName":  {Table: "profiles", Column: "first_name"},
				"name.familyName": {Table: "profiles", Column: "last_name"},
				"name.formatted":  {Table: "profiles", Column: "display"},
			},
			Joins: []postgres.JoinDef{
				{
					Table:      "profiles",
					Condition:  "profiles.account_id = accounts.id",
					JoinType:   "LEFT",
					ForeignKey: "account_id", // used for INSERT / DELETE on "profiles"
					DeleteJoin: true,         // DELETE profiles row before accounts row
				},
			},
		}),

		// ── Map "Group" to a "teams" table (simple rename, no cross-table) ────
		postgres.WithResourceConfig("Group", postgres.ResourceTableConfig{
			Table: "teams",
			FieldMappings: map[string]sqlconn.ColumnRef{
				"id":                {Column: "id"},
				"displayName":       {Column: "display_name"},
				"meta.created":      {Column: "created_at"},
				"meta.lastModified": {Column: "updated_at"},
			},
		}),

		postgres.WithSchemaRegistry(reg),
	)
	if err != nil {
		return err
	}
	defer func() { _ = pgStore.Close(context.Background()) }()

	if err := pgStore.Init(context.Background()); err != nil {
		return err
	}

	// pgStore.Pool() exposes the raw *pgxpool.Pool for queries that are too
	// complex to express with declarative mappings (e.g. multi-step transactions,
	// custom aggregations, or bulk operations).
	pool := pgStore.Pool()
	_ = pool // use pool.Exec / pool.Query / pool.BeginTx as needed

	srv, err := server.New(
		server.WithStore(pgStore),
		server.WithSchemaRegistry(reg),
		server.WithBasePath("/scim/v2"),
		server.WithBearerTokenAuth(func(t string) (bool, error) {
			return t == token, nil
		}),
	)
	if err != nil {
		return err
	}

	log.Printf("SCIM server (existing schema mode) listening on :8080")
	return http.ListenAndServe(":8080", srv)
}
