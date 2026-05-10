package sql_test

import (
	"testing"

	sqlconn "github.com/cerberauth/scimply/connector/sql"
)

func TestMapper_ColumnName(t *testing.T) {
	cases := []struct {
		attrPath   string
		wantCol    string
		wantMapped bool
	}{

		{"userName", "user_name", true},
		{"active", "active", true},

		{"name.familyName", "name_family_name", true},
		{"name.givenName", "name_given_name", true},

		{"meta.created", "meta_created", true},
		{"meta.lastModified", "meta_last_modified", true},

		{"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department", "", false},
		{"urn:ietf:params:scim:schemas:core:2.0:User:userName", "", false},
	}

	mapper := sqlconn.NewMapper()
	for _, tc := range cases {
		t.Run(tc.attrPath, func(t *testing.T) {
			col, ok := mapper.ColumnName(tc.attrPath)
			if ok != tc.wantMapped {
				t.Errorf("ColumnName(%q): ok = %v, want %v", tc.attrPath, ok, tc.wantMapped)
			}
			if col != tc.wantCol {
				t.Errorf("ColumnName(%q): col = %q, want %q", tc.attrPath, col, tc.wantCol)
			}
		})
	}
}

func TestMapper_CustomMapping(t *testing.T) {
	mapper := sqlconn.NewMapper()
	mapper.CustomMappings["userName"] = "login"

	col, ok := mapper.ColumnName("userName")
	if !ok {
		t.Fatal("expected ok = true for custom mapping")
	}
	if col != "login" {
		t.Errorf("col = %q, want %q", col, "login")
	}

	col2, ok2 := mapper.ColumnName("active")
	if !ok2 {
		t.Fatal("expected ok = true for active")
	}
	if col2 != "active" {
		t.Errorf("col = %q, want %q", col2, "active")
	}
}

func TestIsExtension(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"userName", false},
		{"name.familyName", false},
		{"urn:ietf:params:scim:schemas:core:2.0:User:userName", true},
		{"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department", true},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := sqlconn.IsExtension(tc.path)
			if got != tc.want {
				t.Errorf("IsExtension(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestCamelToSnake_viaColumnName(t *testing.T) {

	cases := []struct {
		attr string
		want string
	}{
		{"id", "id"},
		{"active", "active"},
		{"userName", "user_name"},
		{"familyName", "family_name"},
		{"displayName", "display_name"},
		{"externalId", "external_id"},
		{"lastModified", "last_modified"},
	}
	mapper := sqlconn.NewMapper()
	for _, tc := range cases {
		t.Run(tc.attr, func(t *testing.T) {
			got, ok := mapper.ColumnName(tc.attr)
			if !ok {
				t.Fatalf("ColumnName(%q) returned not-ok", tc.attr)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
