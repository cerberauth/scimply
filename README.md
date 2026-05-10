<div align="center">

# scimply

**A complete SCIM 2.0 (and 1.1) server library for Go — RFC-compliant, zero-dependency core, and multiple backends.**

[![Join Discord](https://img.shields.io/discord/1242773130137833493?label=Discord&style=for-the-badge)](https://www.cerberauth.com/community)
[![Build](https://img.shields.io/github/actions/workflow/status/cerberauth/scimply/ci.yml?branch=main&label=build&style=for-the-badge)](https://github.com/cerberauth/scimply/actions/workflows/ci.yml)
![Latest version](https://img.shields.io/github/v/release/cerberauth/scimply?sort=semver&style=for-the-badge)
[![Coverage](https://img.shields.io/codecov/c/gh/cerberauth/scimply?style=for-the-badge)](https://codecov.io/gh/cerberauth/scimply)
[![Go Report Card](https://goreportcard.com/badge/github.com/cerberauth/scimply?style=for-the-badge)](https://goreportcard.com/report/github.com/cerberauth/scimply)
[![GoDoc](https://img.shields.io/badge/godoc-reference-5272B4.svg?style=for-the-badge)](https://pkg.go.dev/github.com/cerberauth/scimply)
[![Stars](https://img.shields.io/github/stars/cerberauth/scimply?style=for-the-badge)](https://github.com/cerberauth/scimply)
[![License](https://img.shields.io/github/license/cerberauth/scimply?style=for-the-badge)](https://github.com/cerberauth/scimply/blob/main/LICENSE)

</div>

---

A complete SCIM 2.0 (and 1.1) server library for Go.

## Features

- **Full SCIM 2.0** (RFC 7642, RFC 7643, RFC 7644) implementation
- **SCIM 1.1 support** via a conversion layer
- **Zero-dependency core** — `schema/`, `resource/`, `protocol/`, `store/`, `server/` use only the Go standard library
- **Filter parser** — recursive-descent ABNF parser for the full RFC 7644 filter grammar
- **PATCH engine** — complete RFC 7644 §3.5.2 semantics with atomicity
- **Multiple backends** — in-memory, PostgreSQL, MySQL/MariaDB, MongoDB, SCIM proxy
- **Compliance test suite** — reusable against any SCIM server
- **Structured audit logging**

## Quick Start

```go
package main

import (
    "log"
    "net/http"

    "github.com/cerberauth/scimply/schema"
    "github.com/cerberauth/scimply/server"
    "github.com/cerberauth/scimply/store"
)

func main() {
    reg := schema.NewRegistry()
    reg.RegisterDefaults() // User, Group, EnterpriseUser

    srv, err := server.New(
        server.WithStore(store.NewMemoryStore()),
        server.WithSchemaRegistry(reg),
        server.WithBasePath("/scim/v2"),
        server.WithBearerTokenAuth(func(token string) (bool, error) {
            return token == "my-secret-token", nil
        }),
    )
    if err != nil {
        log.Fatal(err)
    }

    log.Println("SCIM 2.0 server listening on :8080")
    log.Fatal(http.ListenAndServe(":8080", srv))
}
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/scim/v2/ServiceProviderConfig` | Server capabilities |
| GET | `/scim/v2/ResourceTypes` | Supported resource types |
| GET | `/scim/v2/Schemas` | Schema definitions |
| GET | `/scim/v2/Users` | List users (supports filter, sort, pagination) |
| POST | `/scim/v2/Users` | Create user |
| GET | `/scim/v2/Users/{id}` | Get user |
| PUT | `/scim/v2/Users/{id}` | Replace user |
| PATCH | `/scim/v2/Users/{id}` | Patch user |
| DELETE | `/scim/v2/Users/{id}` | Delete user |
| POST | `/scim/v2/Users/.search` | Query via POST body |
| POST | `/scim/v2/Bulk` | Bulk operations |
| *(same for Groups)* | | |

## Backends

### In-Memory (development and testing)

```go
store.NewMemoryStore()
```

### PostgreSQL

```go
import "github.com/cerberauth/scimply/connector/postgres"

pgStore, err := postgres.New(
    postgres.WithDSN("postgres://user:pass@localhost:5432/scimdb"),
    postgres.WithAutoMigrate(true),
    postgres.WithTablePrefix("scim_"),
    postgres.WithSchemaRegistry(reg),
)
if err != nil { log.Fatal(err) }
defer pgStore.Close(context.Background())
if err := pgStore.Init(context.Background()); err != nil { log.Fatal(err) }
```

### MySQL / MariaDB

```go
import "github.com/cerberauth/scimply/connector/mysql"

myStore, err := mysql.New(
    mysql.WithDSN("user:pass@tcp(localhost:3306)/scimdb?parseTime=true"),
    mysql.WithAutoMigrate(true),
)
```

### MongoDB

```go
import "github.com/cerberauth/scimply/connector/mongodb"

mgStore, err := mongodb.New(
    mongodb.WithURI("mongodb://localhost:27017"),
    mongodb.WithDatabase("scimdb"),
    mongodb.WithAutoMigrate(true),
)
```

### Table & Field Mapping (existing schemas)

All SQL connectors support mapping each SCIM resource type to a specific table and each SCIM attribute to a specific column. Columns can live in different tables — declare JOINs with `JoinDef`. MongoDB supports mapping to a specific collection and arbitrary BSON field paths, with optional cross-collection `$lookup`.

When `FieldMappings` is set for a resource type the connector enters **column mode**: it reads and writes individual columns instead of a JSON blob, and `AutoMigrate` is skipped for that type.

```go
import (
    sqlconn "github.com/cerberauth/scimply/connector/sql"
    "github.com/cerberauth/scimply/connector/postgres"
)

pgStore, err := postgres.New(
    postgres.WithDSN(dsn),

    // Map "User" to existing tables — no data column required.
    postgres.WithResourceConfig("User", postgres.ResourceTableConfig{
        Table: "accounts",
        FieldMappings: map[string]sqlconn.ColumnRef{
            "id":                {Column: "id"},
            "userName":          {Column: "email"},
            "active":            {Column: "is_active"},
            "meta.created":      {Column: "created_at"},
            "meta.lastModified": {Column: "updated_at"},
            // Attributes in a joined table:
            "name.givenName":  {Table: "profiles", Column: "first_name"},
            "name.familyName": {Table: "profiles", Column: "last_name"},
        },
        Joins: []postgres.JoinDef{{
            Table:      "profiles",
            Condition:  "profiles.account_id = accounts.id",
            JoinType:   "LEFT",
            ForeignKey: "account_id", // FK column used for writes
            DeleteJoin: true,
        }},
    }),

    // Convenience helpers build up the same config incrementally:
    //   postgres.WithTableName("Group", "teams")
    //   postgres.WithFieldMapping("Group", "displayName", "name")
    //   postgres.WithTableFieldMapping("Group", "members.value", "team_members", "user_id")
    //   postgres.WithJoin("Group", postgres.JoinDef{...})

    postgres.WithSchemaRegistry(reg),
)

// Access the raw connection pool for queries beyond the declarative config.
pool := pgStore.Pool() // *pgxpool.Pool
```

For MongoDB:

```go
import "github.com/cerberauth/scimply/connector/mongodb"

mgStore, err := mongodb.New(
    mongodb.WithURI(uri),
    mongodb.WithDatabase("myapp"),
    // Map "User" to the "users" collection with custom field paths.
    mongodb.WithResourceConfig("User", mongodb.ResourceCollectionConfig{
        Collection: "users",
        FieldMappings: map[string]string{
            "id":       "_id",
            "userName": "email",
            "active":   "account.active",
        },
        // Cross-collection lookup for 1:1 profile documents:
        Lookups: []mongodb.LookupConfig{{
            From: "profiles", LocalField: "_id", ForeignField: "userId", As: "profile",
            FieldMappings: map[string]string{
                "name.givenName":  "profile.firstName",
                "name.familyName": "profile.lastName",
            },
        }},
    }),
)

db := mgStore.Database() // *mongo.Database — escape hatch for complex queries
```

### SCIM-to-SCIM Proxy

```go
import scimclient "github.com/cerberauth/scimply/connector/scim"

upstream, err := scimclient.New(
    scimclient.WithBaseURL("https://api.example.com/scim/v2"),
    scimclient.WithBearerToken("upstream-token"),
)
```

## Filter Support

Full RFC 7644 filter syntax:

```
GET /scim/v2/Users?filter=userName eq "bjensen"
GET /scim/v2/Users?filter=emails[type eq "work" and value co "@example.com"]
GET /scim/v2/Users?filter=active eq true and userType eq "Employee"
GET /scim/v2/Users?filter=not (userName eq "bjensen")
```

## PATCH Operations

Full RFC 7644 §3.5.2 PATCH semantics:

```json
PATCH /scim/v2/Users/123
{
  "schemas": ["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
  "Operations": [
    {"op": "replace", "path": "active", "value": false},
    {"op": "add", "path": "emails", "value": [{"type": "home", "value": "home@example.com"}]},
    {"op": "replace", "path": "emails[type eq \"work\"].value", "value": "new@example.com"},
    {"op": "remove", "path": "members[value eq \"user-456\"]"}
  ]
}
```

## SCIM 1.1

The library supports SCIM 1.1 via a conversion layer in the `v1/` package. Resources are converted to/from the internal SCIM 2.0 representation at the boundary.

Key 1.1 differences handled:
- Schema URIs: `urn:scim:schemas:core:1.0` ↔ `urn:ietf:params:scim:schemas:core:2.0:User`
- Service provider endpoint: `/ServiceProviderConfigs` (plural)
- Error format: `{"Errors": [{"code": "404", "description": "..."}]}`
- Simplified filter operators (no `ne`, `ew`, `not`, `[]`)

## Compliance Test Suite

Run the standard SCIM 2.0 compliance suite against any server:

```go
import "github.com/cerberauth/scimply/compliance"

func TestMyServer(t *testing.T) {
    srv := startMyServer(t)
    compliance.RunSuite(t, compliance.SuiteConfig{
        BaseURL:    srv.URL + "/scim/v2",
        AuthHeader: "Bearer test-token",
    })
}
```

## License

This repository is licensed under the [MIT License](https://github.com/cerberauth/scimply/blob/main/LICENSE) @ [CerberAuth](https://www.cerberauth.com/). You are free to use, modify, and distribute the contents of this repository for educational and testing purposes.
