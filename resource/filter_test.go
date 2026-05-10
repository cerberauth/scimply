package resource

import (
	"testing"
)

func TestParseFilter_Table(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, expr FilterExpression)
	}{
		{
			name:  "simple eq string",
			input: `userName eq "bjensen"`,
			check: func(t *testing.T, expr FilterExpression) {
				ae, ok := expr.(*AttrExpression)
				if !ok {
					t.Fatalf("expected *AttrExpression, got %T", expr)
				}
				if ae.Path.AttributeName != "userName" {
					t.Errorf("attrName: got %q", ae.Path.AttributeName)
				}
				if ae.Operator != OpEq {
					t.Errorf("op: got %v", ae.Operator)
				}
				if ae.Value != "bjensen" {
					t.Errorf("value: got %v", ae.Value)
				}
			},
		},
		{
			name:  "sub-attribute co",
			input: `name.familyName co "O'Malley"`,
			check: func(t *testing.T, expr FilterExpression) {
				ae, ok := expr.(*AttrExpression)
				if !ok {
					t.Fatalf("expected *AttrExpression, got %T", expr)
				}
				if ae.Path.AttributeName != "name" {
					t.Errorf("attrName: got %q", ae.Path.AttributeName)
				}
				if ae.Path.SubAttribute != "familyName" {
					t.Errorf("subAttr: got %q", ae.Path.SubAttribute)
				}
				if ae.Operator != OpCo {
					t.Errorf("op: got %v", ae.Operator)
				}
			},
		},
		{
			name:  "sw operator",
			input: `userName sw "J"`,
			check: func(t *testing.T, expr FilterExpression) {
				ae := expr.(*AttrExpression)
				if ae.Operator != OpSw {
					t.Errorf("expected sw, got %v", ae.Operator)
				}
				if ae.Value != "J" {
					t.Errorf("value: got %v", ae.Value)
				}
			},
		},
		{
			name:  "pr operator",
			input: `title pr`,
			check: func(t *testing.T, expr FilterExpression) {
				ae, ok := expr.(*AttrExpression)
				if !ok {
					t.Fatalf("expected *AttrExpression, got %T", expr)
				}
				if ae.Operator != OpPr {
					t.Errorf("expected pr, got %v", ae.Operator)
				}
				if ae.Value != nil {
					t.Errorf("expected nil value for pr, got %v", ae.Value)
				}
			},
		},
		{
			name:  "logical and",
			input: `userName eq "bjensen" and name.familyName eq "Jensen"`,
			check: func(t *testing.T, expr FilterExpression) {
				le, ok := expr.(*LogicalExpression)
				if !ok {
					t.Fatalf("expected *LogicalExpression, got %T", expr)
				}
				if le.Op != LogicalAnd {
					t.Errorf("expected And, got %v", le.Op)
				}
			},
		},
		{
			name:  "logical or",
			input: `userName eq "bjensen" or userName eq "jsmith"`,
			check: func(t *testing.T, expr FilterExpression) {
				le, ok := expr.(*LogicalExpression)
				if !ok {
					t.Fatalf("expected *LogicalExpression, got %T", expr)
				}
				if le.Op != LogicalOr {
					t.Errorf("expected Or, got %v", le.Op)
				}
			},
		},
		{
			name:  "not expression",
			input: `not (userName eq "bjensen")`,
			check: func(t *testing.T, expr FilterExpression) {
				ne, ok := expr.(*NotExpression)
				if !ok {
					t.Fatalf("expected *NotExpression, got %T", expr)
				}
				if _, ok := ne.Inner.(*AttrExpression); !ok {
					t.Errorf("expected inner *AttrExpression, got %T", ne.Inner)
				}
			},
		},
		{
			name:  "value path with and",
			input: `emails[type eq "work" and value co "@example.com"]`,
			check: func(t *testing.T, expr FilterExpression) {
				vp, ok := expr.(*ValuePathExpression)
				if !ok {
					t.Fatalf("expected *ValuePathExpression, got %T", expr)
				}
				if vp.Path.AttributeName != "emails" {
					t.Errorf("attrName: got %q", vp.Path.AttributeName)
				}
				if _, ok := vp.Filter.(*LogicalExpression); !ok {
					t.Errorf("expected inner *LogicalExpression, got %T", vp.Filter)
				}
			},
		},
		{
			name:  "value path sub-attribute path",
			input: `emails[type eq "work"].value`,

			wantErr: true,
		},
		{
			name:  "schema URI prefix",
			input: `urn:ietf:params:scim:schemas:core:2.0:User:userName eq "bjensen"`,
			check: func(t *testing.T, expr FilterExpression) {
				ae, ok := expr.(*AttrExpression)
				if !ok {
					t.Fatalf("expected *AttrExpression, got %T", expr)
				}
				if ae.Path.AttributeName != "userName" {
					t.Errorf("attrName: got %q", ae.Path.AttributeName)
				}
				if ae.Path.Schema == "" {
					t.Error("expected non-empty Schema")
				}
			},
		},
		{
			name:  "gt datetime",
			input: `meta.lastModified gt "2011-05-13T04:42:34Z"`,
			check: func(t *testing.T, expr FilterExpression) {
				ae, ok := expr.(*AttrExpression)
				if !ok {
					t.Fatalf("expected *AttrExpression, got %T", expr)
				}
				if ae.Operator != OpGt {
					t.Errorf("expected gt, got %v", ae.Operator)
				}
			},
		},
		{
			name:  "boolean true",
			input: `active eq true`,
			check: func(t *testing.T, expr FilterExpression) {
				ae, ok := expr.(*AttrExpression)
				if !ok {
					t.Fatalf("expected *AttrExpression, got %T", expr)
				}
				if ae.Value != true {
					t.Errorf("expected true, got %v", ae.Value)
				}
			},
		},
		{
			name:  "boolean false",
			input: `active eq false`,
			check: func(t *testing.T, expr FilterExpression) {
				ae, ok := expr.(*AttrExpression)
				if !ok {
					t.Fatalf("expected *AttrExpression, got %T", expr)
				}
				if ae.Value != false {
					t.Errorf("expected false, got %v", ae.Value)
				}
			},
		},
		{
			name:  "grouped or with and precedence",
			input: `(userName eq "a" or userName eq "b") and active eq true`,
			check: func(t *testing.T, expr FilterExpression) {
				le, ok := expr.(*LogicalExpression)
				if !ok {
					t.Fatalf("expected *LogicalExpression, got %T", expr)
				}
				if le.Op != LogicalAnd {
					t.Errorf("expected top-level AND, got %v", le.Op)
				}

				innerOr, ok := le.Left.(*LogicalExpression)
				if !ok {
					t.Fatalf("expected inner *LogicalExpression (or), got %T", le.Left)
				}
				if innerOr.Op != LogicalOr {
					t.Errorf("expected inner OR, got %v", innerOr.Op)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := ParseFilter(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, expr)
			}
		})
	}
}

func TestParseFilter_ValuePathSubAttrAsFilter(t *testing.T) {

	_, err := ParseFilter(`emails[type eq "work"].value`)
	if err == nil {
		t.Log("Note: emails[...].value as standalone filter may or may not be an error depending on interpretation; this is not a hard requirement")
	}
}

func TestParseFilter_Errors(t *testing.T) {
	cases := []string{
		"",
		"userName",
		`userName xyz "bjensen"`,
		`(userName eq "bjensen"`,
		`userName eq`,
		`not userName eq "bjensen"`,
	}
	for _, tc := range cases {
		_, err := ParseFilter(tc)
		if err == nil {
			t.Errorf("expected error for %q, got nil", tc)
		}
	}
}

func TestCompareOp_String(t *testing.T) {
	cases := []struct {
		op   CompareOp
		want string
	}{
		{OpEq, "eq"},
		{OpNe, "ne"},
		{OpCo, "co"},
		{OpSw, "sw"},
		{OpEw, "ew"},
		{OpGt, "gt"},
		{OpGe, "ge"},
		{OpLt, "lt"},
		{OpLe, "le"},
		{OpPr, "pr"},
	}
	for _, tc := range cases {
		if got := tc.op.String(); got != tc.want {
			t.Errorf("CompareOp(%d).String() = %q, want %q", tc.op, got, tc.want)
		}
	}
}

func TestParseFilter_NullValue(t *testing.T) {
	expr, err := ParseFilter(`title eq null`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ae, ok := expr.(*AttrExpression)
	if !ok {
		t.Fatalf("expected *AttrExpression, got %T", expr)
	}
	if ae.Value != nil {
		t.Errorf("expected nil value, got %v", ae.Value)
	}
}

func TestParseFilter_CaseInsensitiveOp(t *testing.T) {
	cases := []string{
		`userName EQ "bjensen"`,
		`userName Eq "bjensen"`,
		`userName eq "bjensen"`,
	}
	for _, tc := range cases {
		expr, err := ParseFilter(tc)
		if err != nil {
			t.Errorf("input %q: unexpected error: %v", tc, err)
			continue
		}
		ae, ok := expr.(*AttrExpression)
		if !ok || ae.Operator != OpEq {
			t.Errorf("input %q: expected eq operator", tc)
		}
	}
}

func TestParseFilter_NumberValue(t *testing.T) {
	expr, err := ParseFilter(`score gt 42.5`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ae, ok := expr.(*AttrExpression)
	if !ok {
		t.Fatalf("expected *AttrExpression")
	}
	if ae.Value.(float64) != 42.5 {
		t.Errorf("expected 42.5, got %v", ae.Value)
	}
}
