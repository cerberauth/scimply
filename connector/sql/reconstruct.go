package sql

import (
	"strings"
	"time"

	"github.com/cerberauth/scimply/resource"
)

// ResourceAttrMap builds a flat map of SCIM attribute paths to values from r.
// Nested attributes are flattened with dot notation (e.g. "name.givenName").
// Special SCIM fields (id, externalId, meta.*) are included with their standard paths.
func ResourceAttrMap(r *resource.Resource) map[string]interface{} {
	m := map[string]interface{}{
		"id":                r.ID,
		"externalId":        r.ExternalID,
		"meta.created":      r.Meta.Created,
		"meta.lastModified": r.Meta.LastModified,
		"meta.version":      r.Meta.Version,
		"meta.location":     r.Meta.Location,
	}
	flattenInto(r.Attributes, "", m)
	return m
}

func flattenInto(attrs map[string]interface{}, prefix string, out map[string]interface{}) {
	for k, v := range attrs {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if sub, ok := v.(map[string]interface{}); ok {
			flattenInto(sub, key, out)
		} else {
			out[key] = v
		}
	}
}

// ReconstructResource builds a Resource from a flat map of SCIM attribute paths to values.
// Keys are SCIM attribute paths; special paths (id, externalId, meta.*) map to Resource
// struct fields; all others go into Attributes using dot notation for nesting.
func ReconstructResource(attrs map[string]interface{}, resourceType string) *resource.Resource {
	r := &resource.Resource{
		Attributes: make(map[string]interface{}),
	}
	r.Meta.ResourceType = resourceType
	for path, val := range attrs {
		if val == nil {
			continue
		}
		applyAttr(r, path, val)
	}
	return r
}

func applyAttr(r *resource.Resource, path string, val interface{}) {
	switch strings.ToLower(path) {
	case "id":
		r.ID = asString(val)
	case "externalid":
		r.ExternalID = asString(val)
	case "meta.created":
		r.Meta.Created = asTime(val)
	case "meta.lastmodified":
		r.Meta.LastModified = asTime(val)
	case "meta.version":
		r.Meta.Version = asString(val)
	case "meta.location":
		r.Meta.Location = asString(val)
	default:
		setNested(r.Attributes, path, val)
	}
}

func setNested(attrs map[string]interface{}, path string, val interface{}) {
	dot := strings.IndexByte(path, '.')
	if dot < 0 {
		attrs[path] = val
		return
	}
	parent, rest := path[:dot], path[dot+1:]
	sub, ok := attrs[parent].(map[string]interface{})
	if !ok {
		sub = make(map[string]interface{})
	}
	setNested(sub, rest, val)
	attrs[parent] = sub
}

func asString(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	}
	return ""
}

func asTime(v interface{}) time.Time {
	if t, ok := v.(time.Time); ok {
		return t
	}
	return time.Time{}
}
