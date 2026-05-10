package resource

import (
	"testing"
)

func makeUser(attrs map[string]interface{}) *Resource {
	r := &Resource{
		Schemas:    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		Attributes: attrs,
	}
	return r
}

func mustParseFilter(t *testing.T, s string) FilterExpression {
	t.Helper()
	expr, err := ParseFilter(s)
	if err != nil {
		t.Fatalf("ParseFilter(%q): %v", s, err)
	}
	return expr
}

func TestEvalFilter_SimpleEq(t *testing.T) {
	r := makeUser(map[string]interface{}{"userName": "bjensen"})
	if !EvalFilter(mustParseFilter(t, `userName eq "bjensen"`), r) {
		t.Error("expected match")
	}
	if EvalFilter(mustParseFilter(t, `userName eq "jsmith"`), r) {
		t.Error("expected no match")
	}
}

func TestEvalFilter_Ne(t *testing.T) {
	r := makeUser(map[string]interface{}{"userName": "bjensen"})
	if !EvalFilter(mustParseFilter(t, `userName ne "jsmith"`), r) {
		t.Error("expected match for ne")
	}
	if EvalFilter(mustParseFilter(t, `userName ne "bjensen"`), r) {
		t.Error("expected no match for ne with same value")
	}
}

func TestEvalFilter_Co(t *testing.T) {
	r := makeUser(map[string]interface{}{"userName": "bjensen"})

	if !EvalFilter(mustParseFilter(t, `userName co "jens"`), r) {
		t.Error("expected match for co")
	}
	if EvalFilter(mustParseFilter(t, `userName co "xyz"`), r) {
		t.Error("expected no match for co")
	}
}

func TestEvalFilter_Sw(t *testing.T) {
	r := makeUser(map[string]interface{}{"userName": "bjensen"})
	if !EvalFilter(mustParseFilter(t, `userName sw "bj"`), r) {
		t.Error("expected match for sw")
	}
	if EvalFilter(mustParseFilter(t, `userName sw "jen"`), r) {
		t.Error("expected no match for sw")
	}
}

func TestEvalFilter_Ew(t *testing.T) {
	r := makeUser(map[string]interface{}{"userName": "bjensen"})
	if !EvalFilter(mustParseFilter(t, `userName ew "sen"`), r) {
		t.Error("expected match for ew")
	}
	if EvalFilter(mustParseFilter(t, `userName ew "bj"`), r) {
		t.Error("expected no match for ew")
	}
}

func TestEvalFilter_Pr_Present(t *testing.T) {
	r := makeUser(map[string]interface{}{"title": "Engineer"})
	if !EvalFilter(mustParseFilter(t, `title pr`), r) {
		t.Error("expected match for pr on present attribute")
	}
}

func TestEvalFilter_Pr_Absent(t *testing.T) {
	r := makeUser(map[string]interface{}{})
	if EvalFilter(mustParseFilter(t, `title pr`), r) {
		t.Error("expected no match for pr on absent attribute")
	}
}

func TestEvalFilter_MultiValuedAny(t *testing.T) {
	r := makeUser(map[string]interface{}{
		"emails": []interface{}{
			map[string]interface{}{"value": "home@example.com", "type": "home"},
			map[string]interface{}{"value": "work@example.com", "type": "work"},
		},
	})

	expr, _ := ParseFilter(`emails[type eq "work"]`)
	if !EvalFilter(expr, r) {
		t.Error("expected match on multi-valued attribute")
	}
	expr2, _ := ParseFilter(`emails[type eq "other"]`)
	if EvalFilter(expr2, r) {
		t.Error("expected no match")
	}
}

func TestEvalFilter_BooleanTrue(t *testing.T) {
	r := makeUser(map[string]interface{}{"active": true})
	if !EvalFilter(mustParseFilter(t, `active eq true`), r) {
		t.Error("expected match for active eq true")
	}
	if EvalFilter(mustParseFilter(t, `active eq false`), r) {
		t.Error("expected no match for active eq false")
	}
}

func TestEvalFilter_BooleanFalse(t *testing.T) {
	r := makeUser(map[string]interface{}{"active": false})
	if !EvalFilter(mustParseFilter(t, `active eq false`), r) {
		t.Error("expected match for active eq false")
	}
}

func TestEvalFilter_NullComparison(t *testing.T) {
	r := makeUser(map[string]interface{}{"title": nil})
	if !EvalFilter(mustParseFilter(t, `title eq null`), r) {
		t.Error("expected match for null eq null")
	}
}

func TestEvalFilter_LogicalAnd(t *testing.T) {
	r := makeUser(map[string]interface{}{"userName": "bjensen", "active": true})
	if !EvalFilter(mustParseFilter(t, `userName eq "bjensen" and active eq true`), r) {
		t.Error("expected match for and")
	}
	if EvalFilter(mustParseFilter(t, `userName eq "bjensen" and active eq false`), r) {
		t.Error("expected no match for and with false second condition")
	}
}

func TestEvalFilter_LogicalOr(t *testing.T) {
	r := makeUser(map[string]interface{}{"userName": "bjensen"})
	if !EvalFilter(mustParseFilter(t, `userName eq "bjensen" or userName eq "jsmith"`), r) {
		t.Error("expected match for or")
	}
	if EvalFilter(mustParseFilter(t, `userName eq "nobody" or userName eq "jsmith"`), r) {
		t.Error("expected no match for or when neither matches")
	}
}

func TestEvalFilter_Not(t *testing.T) {
	r := makeUser(map[string]interface{}{"userName": "bjensen"})
	if !EvalFilter(mustParseFilter(t, `not (userName eq "jsmith")`), r) {
		t.Error("expected match for not")
	}
	if EvalFilter(mustParseFilter(t, `not (userName eq "bjensen")`), r) {
		t.Error("expected no match for not when inner matches")
	}
}

func TestEvalFilter_ValuePathExpression(t *testing.T) {
	r := makeUser(map[string]interface{}{
		"emails": []interface{}{
			map[string]interface{}{"value": "work@example.com", "type": "work", "primary": true},
			map[string]interface{}{"value": "home@example.com", "type": "home"},
		},
	})
	expr, err := ParseFilter(`emails[type eq "work" and value co "@example.com"]`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !EvalFilter(expr, r) {
		t.Error("expected match for value path with and filter")
	}
}

func TestEvalFilter_NestedSubAttribute(t *testing.T) {
	r := makeUser(map[string]interface{}{
		"name": map[string]interface{}{
			"familyName": "Jensen",
			"givenName":  "Barbara",
		},
	})
	if !EvalFilter(mustParseFilter(t, `name.familyName eq "Jensen"`), r) {
		t.Error("expected match on sub-attribute")
	}
	if EvalFilter(mustParseFilter(t, `name.familyName eq "Smith"`), r) {
		t.Error("expected no match on sub-attribute")
	}
}

func TestEvalFilter_NilResource(t *testing.T) {
	expr := mustParseFilter(t, `userName eq "bjensen"`)
	if EvalFilter(expr, nil) {
		t.Error("expected false for nil resource")
	}
}

func TestEvalFilter_CaseInsensitiveAttrName(t *testing.T) {
	r := makeUser(map[string]interface{}{"userName": "bjensen"})

	if !EvalFilter(mustParseFilter(t, `username eq "bjensen"`), r) {
		t.Error("expected case-insensitive attribute name match")
	}
}
