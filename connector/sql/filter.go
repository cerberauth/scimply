package sql

import (
	"fmt"
	"strings"

	"github.com/cerberauth/scimply/resource"
)

type FilterResult struct {
	Clause string
	Args   []interface{}
	Err    error
}

type Dialect int

const (
	DialectPostgres Dialect = iota
	DialectMySQL
)

// translationState carries mutable context through the recursive filter translation.
// args accumulates positional query parameters in order so callers can pass them
// to sql.DB.Query. tableAlias is prefixed to every column reference to support
// multi-table queries (e.g. "u.email" instead of "email").
type translationState struct {
	args       []interface{}
	dialect    Dialect
	mapper     *Mapper
	tableAlias string
}

// placeholder appends val to the query argument list and returns the
// dialect-appropriate placeholder token. PostgreSQL uses positional placeholders
// ($1, $2, …) while MySQL uses anonymous "?" markers.
func (s *translationState) placeholder(val interface{}) string {
	s.args = append(s.args, val)
	n := len(s.args)
	if s.dialect == DialectMySQL {
		return "?"
	}
	return fmt.Sprintf("$%d", n)
}

func (s *translationState) colRef(col string) string {
	if s.tableAlias != "" {
		return s.tableAlias + "." + col
	}
	return col
}

func TranslateFilter(expr resource.FilterExpression, mapper *Mapper, dialect Dialect, tableAlias string) FilterResult {
	if expr == nil {
		return FilterResult{}
	}
	state := &translationState{
		dialect:    dialect,
		mapper:     mapper,
		tableAlias: tableAlias,
	}
	clause, err := translateExpr(expr, state)
	return FilterResult{Clause: clause, Args: state.args, Err: err}
}

func translateExpr(expr resource.FilterExpression, s *translationState) (string, error) {
	switch e := expr.(type) {
	case *resource.AttrExpression:
		return translateAttrExpr(e, s)
	case *resource.LogicalExpression:
		return translateLogicalExpr(e, s)
	case *resource.NotExpression:
		return translateNotExpr(e, s)
	case *resource.ValuePathExpression:
		return translateValuePathExpr(e, s)
	default:
		return "", fmt.Errorf("unsupported filter expression type %T", expr)
	}
}

func translateAttrExpr(e *resource.AttrExpression, s *translationState) (string, error) {
	attrPath := e.Path.String()
	mref, ok := s.mapper.Ref(attrPath)
	if !ok {
		return "", fmt.Errorf("attribute %q cannot be mapped to a column", attrPath)
	}
	var ref string
	if mref.Table != "" {
		ref = mref.Table + "." + mref.Column
	} else {
		ref = s.colRef(mref.Column)
	}

	switch e.Operator {
	case resource.OpPr:
		return ref + " IS NOT NULL", nil
	case resource.OpEq:
		p := s.placeholder(e.Value)
		return ref + " = " + p, nil
	case resource.OpNe:
		p := s.placeholder(e.Value)
		return ref + " != " + p, nil
	case resource.OpGt:
		p := s.placeholder(e.Value)
		return ref + " > " + p, nil
	case resource.OpGe:
		p := s.placeholder(e.Value)
		return ref + " >= " + p, nil
	case resource.OpLt:
		p := s.placeholder(e.Value)
		return ref + " < " + p, nil
	case resource.OpLe:
		p := s.placeholder(e.Value)
		return ref + " <= " + p, nil
	case resource.OpCo:
		p := s.placeholder(e.Value)
		if s.dialect == DialectMySQL {
			return ref + " LIKE CONCAT('%', " + p + ", '%')", nil
		}
		return ref + " LIKE '%' || " + p + " || '%'", nil
	case resource.OpSw:
		p := s.placeholder(e.Value)
		if s.dialect == DialectMySQL {
			return ref + " LIKE CONCAT(" + p + ", '%')", nil
		}
		return ref + " LIKE " + p + " || '%'", nil
	case resource.OpEw:
		p := s.placeholder(e.Value)
		if s.dialect == DialectMySQL {
			return ref + " LIKE CONCAT('%', " + p + ")", nil
		}
		return ref + " LIKE '%' || " + p, nil
	default:
		return "", fmt.Errorf("unsupported operator %v", e.Operator)
	}
}

func translateLogicalExpr(e *resource.LogicalExpression, s *translationState) (string, error) {
	left, err := translateExpr(e.Left, s)
	if err != nil {
		return "", err
	}
	right, err := translateExpr(e.Right, s)
	if err != nil {
		return "", err
	}
	op := "AND"
	if e.Op == resource.LogicalOr {
		op = "OR"
	}
	return "(" + left + " " + op + " " + right + ")", nil
}

func translateNotExpr(e *resource.NotExpression, s *translationState) (string, error) {
	inner, err := translateExpr(e.Inner, s)
	if err != nil {
		return "", err
	}
	return "NOT (" + inner + ")", nil
}

// translateValuePathExpr converts a SCIM value-path filter (e.g. emails[type eq "work"])
// into an SQL EXISTS sub-query. The approach:
//  1. Derive a short table alias from the first letter of the attribute name.
//  2. Create a child translationState that shares the parent's args slice so
//     parameter positions remain globally consistent across the outer query.
//  3. After the inner expression is translated, copy the (possibly extended)
//     args back to the parent state.
//  4. Assume the related table is named <attribute>s (e.g. "emails") and that
//     it has a resource_id foreign key referencing the parent row.
func translateValuePathExpr(e *resource.ValuePathExpression, s *translationState) (string, error) {
	attrName := strings.ToLower(e.Path.AttributeName)

	subAlias := attrName[:1]
	innerMapper := s.mapper
	subState := &translationState{
		dialect:    s.dialect,
		mapper:     innerMapper,
		tableAlias: subAlias,
		args:       s.args, // share the arg list so $N positions are contiguous
	}

	inner, err := translateExpr(e.Filter, subState)
	s.args = subState.args // propagate any new args back to parent
	if err != nil {
		return "", err
	}

	subTable := attrName + "s"
	var parentID string
	if s.tableAlias != "" {
		parentID = s.tableAlias + ".id"
	} else {
		parentID = "id"
	}

	return fmt.Sprintf(
		"EXISTS (SELECT 1 FROM %s %s WHERE %s.resource_id = %s AND %s)",
		subTable, subAlias, subAlias, parentID, inner,
	), nil
}
