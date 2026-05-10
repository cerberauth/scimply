package sql_test

import (
	"testing"

	sqlconn "github.com/cerberauth/scimply/connector/sql"
	"github.com/cerberauth/scimply/resource"
)

func mustParseFilter(t *testing.T, s string) resource.FilterExpression {
	t.Helper()
	expr, err := resource.ParseFilter(s)
	if err != nil {
		t.Fatalf("ParseFilter(%q): %v", s, err)
	}
	return expr
}

func TestTranslateFilter_SimpleStringEq(t *testing.T) {
	mapper := sqlconn.NewMapper()
	expr := mustParseFilter(t, `userName eq "jdoe"`)
	result := sqlconn.TranslateFilter(expr, mapper, sqlconn.DialectPostgres, "u")
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	want := "u.user_name = $1"
	if result.Clause != want {
		t.Errorf("Clause = %q, want %q", result.Clause, want)
	}
	if len(result.Args) != 1 || result.Args[0] != "jdoe" {
		t.Errorf("Args = %v, want [\"jdoe\"]", result.Args)
	}
}

func TestTranslateFilter_BooleanEq(t *testing.T) {
	mapper := sqlconn.NewMapper()
	expr := mustParseFilter(t, `active eq true`)
	result := sqlconn.TranslateFilter(expr, mapper, sqlconn.DialectPostgres, "")
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	want := "active = $1"
	if result.Clause != want {
		t.Errorf("Clause = %q, want %q", result.Clause, want)
	}
	if len(result.Args) != 1 || result.Args[0] != true {
		t.Errorf("Args = %v, want [true]", result.Args)
	}
}

func TestTranslateFilter_ContainsPostgres(t *testing.T) {
	mapper := sqlconn.NewMapper()
	expr := mustParseFilter(t, `userName co "doe"`)
	result := sqlconn.TranslateFilter(expr, mapper, sqlconn.DialectPostgres, "u")
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	want := "u.user_name LIKE '%' || $1 || '%'"
	if result.Clause != want {
		t.Errorf("Clause = %q, want %q", result.Clause, want)
	}
}

func TestTranslateFilter_ContainsMySQL(t *testing.T) {
	mapper := sqlconn.NewMapper()
	expr := mustParseFilter(t, `userName co "doe"`)
	result := sqlconn.TranslateFilter(expr, mapper, sqlconn.DialectMySQL, "u")
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	want := "u.user_name LIKE CONCAT('%', ?, '%')"
	if result.Clause != want {
		t.Errorf("Clause = %q, want %q", result.Clause, want)
	}
}

func TestTranslateFilter_StartsWith(t *testing.T) {
	mapper := sqlconn.NewMapper()
	expr := mustParseFilter(t, `userName sw "j"`)
	result := sqlconn.TranslateFilter(expr, mapper, sqlconn.DialectPostgres, "")
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	want := "user_name LIKE $1 || '%'"
	if result.Clause != want {
		t.Errorf("Clause = %q, want %q", result.Clause, want)
	}
}

func TestTranslateFilter_EndsWith(t *testing.T) {
	mapper := sqlconn.NewMapper()
	expr := mustParseFilter(t, `userName ew "doe"`)
	result := sqlconn.TranslateFilter(expr, mapper, sqlconn.DialectPostgres, "")
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	want := "user_name LIKE '%' || $1"
	if result.Clause != want {
		t.Errorf("Clause = %q, want %q", result.Clause, want)
	}
}

func TestTranslateFilter_LogicalAnd(t *testing.T) {
	mapper := sqlconn.NewMapper()
	expr := mustParseFilter(t, `userName eq "jdoe" and active eq true`)
	result := sqlconn.TranslateFilter(expr, mapper, sqlconn.DialectPostgres, "")
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	want := "(user_name = $1 AND active = $2)"
	if result.Clause != want {
		t.Errorf("Clause = %q, want %q", result.Clause, want)
	}
	if len(result.Args) != 2 {
		t.Errorf("len(Args) = %d, want 2", len(result.Args))
	}
}

func TestTranslateFilter_LogicalOr(t *testing.T) {
	mapper := sqlconn.NewMapper()
	expr := mustParseFilter(t, `userName eq "alice" or userName eq "bob"`)
	result := sqlconn.TranslateFilter(expr, mapper, sqlconn.DialectPostgres, "")
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	want := "(user_name = $1 OR user_name = $2)"
	if result.Clause != want {
		t.Errorf("Clause = %q, want %q", result.Clause, want)
	}
}

func TestTranslateFilter_Not(t *testing.T) {
	mapper := sqlconn.NewMapper()
	expr := mustParseFilter(t, `not (active eq true)`)
	result := sqlconn.TranslateFilter(expr, mapper, sqlconn.DialectPostgres, "")
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	want := "NOT (active = $1)"
	if result.Clause != want {
		t.Errorf("Clause = %q, want %q", result.Clause, want)
	}
}

func TestTranslateFilter_Presence(t *testing.T) {
	mapper := sqlconn.NewMapper()
	expr := mustParseFilter(t, `userName pr`)
	result := sqlconn.TranslateFilter(expr, mapper, sqlconn.DialectPostgres, "u")
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	want := "u.user_name IS NOT NULL"
	if result.Clause != want {
		t.Errorf("Clause = %q, want %q", result.Clause, want)
	}
	if len(result.Args) != 0 {
		t.Errorf("len(Args) = %d, want 0", len(result.Args))
	}
}

func TestTranslateFilter_DateTimeGt(t *testing.T) {
	mapper := sqlconn.NewMapper()

	mapper.CustomMappings["meta.created"] = "meta_created"
	expr := mustParseFilter(t, `meta.created gt "2024-01-01T00:00:00Z"`)
	result := sqlconn.TranslateFilter(expr, mapper, sqlconn.DialectPostgres, "")
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	want := "meta_created > $1"
	if result.Clause != want {
		t.Errorf("Clause = %q, want %q", result.Clause, want)
	}
	if len(result.Args) != 1 {
		t.Errorf("len(Args) = %d, want 1", len(result.Args))
	}
}

func TestTranslateFilter_NilExpression(t *testing.T) {
	mapper := sqlconn.NewMapper()
	result := sqlconn.TranslateFilter(nil, mapper, sqlconn.DialectPostgres, "")
	if result.Err != nil {
		t.Errorf("unexpected error for nil expression: %v", result.Err)
	}
	if result.Clause != "" {
		t.Errorf("Clause = %q, want empty string", result.Clause)
	}
}

func TestTranslateFilter_MySQLArgs(t *testing.T) {
	mapper := sqlconn.NewMapper()
	expr := mustParseFilter(t, `userName eq "jdoe" and active eq true`)
	result := sqlconn.TranslateFilter(expr, mapper, sqlconn.DialectMySQL, "")
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	want := "(user_name = ? AND active = ?)"
	if result.Clause != want {
		t.Errorf("Clause = %q, want %q", result.Clause, want)
	}
	if len(result.Args) != 2 {
		t.Errorf("len(Args) = %d, want 2", len(result.Args))
	}
}
