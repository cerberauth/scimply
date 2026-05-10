package resource

import (
	"fmt"
	"math"
	"strings"
)

// EvalFilter is the entry point for evaluating a parsed SCIM filter expression
// against a resource. It converts the resource to a plain map so that the
// recursive evalExpr can work with a uniform data structure regardless of how
// attributes are internally stored.
func EvalFilter(expr FilterExpression, res *Resource) bool {
	if res == nil {
		return false
	}
	return evalExpr(expr, res.ToMap())
}

// evalExpr dispatches to the appropriate evaluator based on the expression node type.
// LogicalExpression uses short-circuit evaluation: AND returns false immediately when
// the left side is false; OR returns true immediately when the left side is true.
// This mirrors standard boolean semantics and avoids unnecessary recursive calls.
func evalExpr(expr FilterExpression, attrs map[string]interface{}) bool {
	switch e := expr.(type) {
	case *AttrExpression:
		return evalAttrExpr(e, attrs)
	case *LogicalExpression:
		left := evalExpr(e.Left, attrs)
		switch e.Op {
		case LogicalAnd:
			if !left {
				return false
			}
			return evalExpr(e.Right, attrs)
		case LogicalOr:
			if left {
				return true
			}
			return evalExpr(e.Right, attrs)
		}
	case *NotExpression:
		return !evalExpr(e.Inner, attrs)
	case *ValuePathExpression:
		return evalValuePath(e, attrs)
	}
	return false
}

// evalAttrExpr evaluates a single attribute comparison against the attrs map.
// For multi-valued attributes (arrays), it performs an ANY match: if at least
// one element satisfies the comparison the expression is true. When a sub-attribute
// is specified (e.g. emails.value), each array element is expected to be a map
// and only the sub-attribute field is extracted for comparison.
func evalAttrExpr(e *AttrExpression, attrs map[string]interface{}) bool {
	val := getAttrValue(attrs, e.Path)

	if e.Operator == OpPr {
		return val != nil
	}

	if arr, ok := toSlice(val); ok {
		for _, elem := range arr {
			var cmpVal interface{}
			if e.Path.SubAttribute != "" {
				if m, ok := elem.(map[string]interface{}); ok {
					cmpVal = lookupKey(m, e.Path.SubAttribute)
				}
			} else {
				cmpVal = elem
			}
			if compareValues(e.Operator, cmpVal, e.Value) {
				return true
			}
		}
		return false
	}

	return compareValues(e.Operator, val, e.Value)
}

// evalValuePath handles the SCIM value-path filter syntax: attr[subFilter].
// It retrieves the named attribute, then evaluates subFilter against each
// element of the array. If the attribute is a single complex object (not an
// array) the filter is evaluated directly against that object's fields.
func evalValuePath(e *ValuePathExpression, attrs map[string]interface{}) bool {
	val := lookupKey(attrs, e.Path.AttributeName)
	if val == nil {
		return false
	}
	arr, ok := toSlice(val)
	if !ok {
		// Attribute is a single complex object, not an array.
		if m, ok := val.(map[string]interface{}); ok {
			return evalExpr(e.Filter, m)
		}
		return false
	}
	for _, elem := range arr {
		if m, ok := elem.(map[string]interface{}); ok {
			if evalExpr(e.Filter, m) {
				return true
			}
		}
	}
	return false
}

func getAttrValue(attrs map[string]interface{}, path AttributePath) interface{} {
	val := lookupKey(attrs, path.AttributeName)
	if val == nil {
		return nil
	}
	if path.SubAttribute == "" {
		return val
	}
	if m, ok := val.(map[string]interface{}); ok {
		return lookupKey(m, path.SubAttribute)
	}
	return val
}

func lookupKey(m map[string]interface{}, key string) interface{} {
	if v, ok := m[key]; ok {
		return v
	}
	lower := strings.ToLower(key)
	for k, v := range m {
		if strings.ToLower(k) == lower {
			return v
		}
	}
	return nil
}

func toSlice(v interface{}) ([]interface{}, bool) {
	if v == nil {
		return nil, false
	}
	switch sv := v.(type) {
	case []interface{}:
		return sv, true
	case []map[string]interface{}:
		out := make([]interface{}, len(sv))
		for i, m := range sv {
			out[i] = m
		}
		return out, true
	case []string:
		out := make([]interface{}, len(sv))
		for i, s := range sv {
			out[i] = s
		}
		return out, true
	}
	return nil, false
}

func compareValues(op CompareOp, attrVal, filterVal interface{}) bool {
	switch op {
	case OpEq:
		return valuesEqual(attrVal, filterVal)
	case OpNe:
		return !valuesEqual(attrVal, filterVal)
	case OpCo:
		a, fa := toStringPair(attrVal, filterVal)
		return strings.Contains(strings.ToLower(a), strings.ToLower(fa))
	case OpSw:
		a, fa := toStringPair(attrVal, filterVal)
		return strings.HasPrefix(strings.ToLower(a), strings.ToLower(fa))
	case OpEw:
		a, fa := toStringPair(attrVal, filterVal)
		return strings.HasSuffix(strings.ToLower(a), strings.ToLower(fa))
	case OpGt:
		return compareOrder(attrVal, filterVal) > 0
	case OpGe:
		return compareOrder(attrVal, filterVal) >= 0
	case OpLt:
		return compareOrder(attrVal, filterVal) < 0
	case OpLe:
		return compareOrder(attrVal, filterVal) <= 0
	case OpPr:
		return attrVal != nil
	}
	return false
}

// valuesEqual compares two SCIM attribute values for equality. The comparison
// strategy depends on the runtime types:
//   - Booleans are compared directly to avoid coercion to strings.
//   - Numbers (any numeric type) are compared as float64 with an epsilon of 1e-15
//     to handle floating-point imprecision (e.g., 1.0 == 1).
//   - Everything else falls back to case-insensitive string comparison, which
//     handles SCIM's case-insensitive string semantics.
func valuesEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	ab, aIsBool := a.(bool)
	bb, bIsBool := b.(bool)
	if aIsBool && bIsBool {
		return ab == bb
	}
	af, aIsFloat := toFloat(a)
	bf, bIsFloat := toFloat(b)
	if aIsFloat && bIsFloat {
		return math.Abs(af-bf) < 1e-15
	}
	return strings.EqualFold(fmt.Sprintf("%v", a), fmt.Sprintf("%v", b))
}

func toStringPair(a, b interface{}) (string, string) {
	return fmt.Sprintf("%v", a), fmt.Sprintf("%v", b)
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	}
	return 0, false
}

func compareOrder(a, b interface{}) int {
	af, aIsFloat := toFloat(a)
	bf, bIsFloat := toFloat(b)
	if aIsFloat && bIsFloat {
		if af < bf {
			return -1
		}
		if af > bf {
			return 1
		}
		return 0
	}
	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)
	return strings.Compare(as, bs)
}
