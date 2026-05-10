package mongodb

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/cerberauth/scimply/resource"
)

func TranslateFilter(expr resource.FilterExpression) (bson.D, error) {
	if expr == nil {
		return bson.D{}, nil
	}
	return translateExpr(expr)
}

func translateExpr(expr resource.FilterExpression) (bson.D, error) {
	switch e := expr.(type) {
	case *resource.AttrExpression:
		return translateAttrExpr(e)
	case *resource.LogicalExpression:
		return translateLogicalExpr(e)
	case *resource.NotExpression:
		return translateNotExpr(e)
	case *resource.ValuePathExpression:
		return translateValuePathExpr(e)
	default:
		return nil, fmt.Errorf("unsupported filter expression type %T", expr)
	}
}

// scimFieldToMongoField translates a SCIM attribute path to the MongoDB field name.
// The SCIM "id" attribute maps to MongoDB's "_id" primary key. Sub-attributes
// (e.g. name.givenName) are represented using MongoDB's dot-notation.
func scimFieldToMongoField(path resource.AttributePath) string {
	name := path.AttributeName

	if strings.EqualFold(name, "id") && path.SubAttribute == "" && path.Schema == "" {
		return "_id"
	}
	if path.SubAttribute != "" {
		return name + "." + path.SubAttribute
	}
	return name
}

func translateAttrExpr(e *resource.AttrExpression) (bson.D, error) {
	field := scimFieldToMongoField(e.Path)

	switch e.Operator {
	case resource.OpEq:
		return bson.D{{Key: field, Value: e.Value}}, nil

	case resource.OpNe:
		return bson.D{{Key: field, Value: bson.D{{Key: "$ne", Value: e.Value}}}}, nil

	case resource.OpCo:
		val, _ := e.Value.(string)
		return bson.D{{Key: field, Value: bson.D{{Key: "$regex", Value: ".*" + val + ".*"}}}}, nil

	case resource.OpSw:
		val, _ := e.Value.(string)
		return bson.D{{Key: field, Value: bson.D{{Key: "$regex", Value: "^" + val}}}}, nil

	case resource.OpEw:
		val, _ := e.Value.(string)
		return bson.D{{Key: field, Value: bson.D{{Key: "$regex", Value: val + "$"}}}}, nil

	case resource.OpGt:
		return bson.D{{Key: field, Value: bson.D{{Key: "$gt", Value: e.Value}}}}, nil

	case resource.OpGe:
		return bson.D{{Key: field, Value: bson.D{{Key: "$gte", Value: e.Value}}}}, nil

	case resource.OpLt:
		return bson.D{{Key: field, Value: bson.D{{Key: "$lt", Value: e.Value}}}}, nil

	case resource.OpLe:
		return bson.D{{Key: field, Value: bson.D{{Key: "$lte", Value: e.Value}}}}, nil

	case resource.OpPr:
		return bson.D{{Key: field, Value: bson.D{
			{Key: "$exists", Value: true},
			{Key: "$ne", Value: nil},
		}}}, nil

	default:
		return nil, fmt.Errorf("unsupported operator %v", e.Operator)
	}
}

func translateLogicalExpr(e *resource.LogicalExpression) (bson.D, error) {
	left, err := translateExpr(e.Left)
	if err != nil {
		return nil, err
	}
	right, err := translateExpr(e.Right)
	if err != nil {
		return nil, err
	}

	switch e.Op {
	case resource.LogicalAnd:
		return bson.D{{Key: "$and", Value: bson.A{left, right}}}, nil
	case resource.LogicalOr:
		return bson.D{{Key: "$or", Value: bson.A{left, right}}}, nil
	default:
		return nil, fmt.Errorf("unsupported logical operator %v", e.Op)
	}
}

// translateNotExpr maps SCIM "not" to MongoDB "$nor" with a single-element array.
// "$nor" returns documents that fail all given conditions, which is equivalent
// to logical NOT for a single condition.
func translateNotExpr(e *resource.NotExpression) (bson.D, error) {
	inner, err := translateExpr(e.Inner)
	if err != nil {
		return nil, err
	}
	return bson.D{{Key: "$nor", Value: bson.A{inner}}}, nil
}

// translateValuePathExpr maps a SCIM value-path filter (e.g. emails[type eq "work"])
// to a MongoDB "$elemMatch" query. "$elemMatch" matches documents where at least
// one array element satisfies all conditions in the sub-filter, mirroring SCIM's
// ANY-match semantics for multi-valued attributes.
func translateValuePathExpr(e *resource.ValuePathExpression) (bson.D, error) {
	innerFilter, err := translateExpr(e.Filter)
	if err != nil {
		return nil, err
	}
	field := e.Path.AttributeName
	return bson.D{{Key: field, Value: bson.D{{Key: "$elemMatch", Value: innerFilter}}}}, nil
}
