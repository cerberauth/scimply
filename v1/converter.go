package v1

import (
	"strings"

	"github.com/cerberauth/scimply/resource"
)

func ToV2(v1Resource map[string]interface{}, resourceTypeName string) (*resource.Resource, error) {
	out := make(map[string]interface{}, len(v1Resource))
	for k, v := range v1Resource {
		out[k] = v
	}

	if schemas, ok := v1Resource["schemas"]; ok {
		out["schemas"] = remapSchemasToV2(schemas, resourceTypeName)
	}

	out = promoteEnterpriseExtension(out)

	fixManagerField(out)

	fixGroupsMembership(out)

	return resource.FromMap(out), nil
}

func FromV2(v2 *resource.Resource, resourceTypeName string) (map[string]interface{}, error) {
	m := v2.ToMap()
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}

	if schemas, ok := m["schemas"]; ok {
		out["schemas"] = remapSchemasToV1(schemas, resourceTypeName)
	}

	out = flattenEnterpriseExtension(out)

	fixManagerFieldToV1(out)

	return out, nil
}

func remapSchemasToV2(schemas interface{}, resourceTypeName string) []string {
	v2URI := v2UserSchemaURI
	if strings.EqualFold(resourceTypeName, "Group") {
		v2URI = v2GroupSchemaURI
	}

	result := []string{v2URI}
	raw, ok := toStringSlice(schemas)
	if !ok {
		return result
	}
	seen := map[string]bool{v2URI: true}
	for _, s := range raw {
		switch s {
		case CoreSchemaURI:

		case EnterpriseSchemaURI:
			if !seen[v2EnterpriseSchemaURI] {
				result = append(result, v2EnterpriseSchemaURI)
				seen[v2EnterpriseSchemaURI] = true
			}
		default:
			if !seen[s] {
				result = append(result, s)
				seen[s] = true
			}
		}
	}
	return result
}

func remapSchemasToV1(schemas interface{}, resourceTypeName string) []string {
	result := []string{CoreSchemaURI}
	raw, ok := toStringSlice(schemas)
	if !ok {
		return result
	}
	seen := map[string]bool{CoreSchemaURI: true}
	for _, s := range raw {
		switch s {
		case v2UserSchemaURI, v2GroupSchemaURI:

		case v2EnterpriseSchemaURI:
			if !seen[EnterpriseSchemaURI] {
				result = append(result, EnterpriseSchemaURI)
				seen[EnterpriseSchemaURI] = true
			}
		default:
			if !seen[s] {
				result = append(result, s)
				seen[s] = true
			}
		}
	}
	return result
}

// promoteEnterpriseExtension moves top-level enterprise attributes from a SCIM v1
// resource into the nested v2 enterprise extension object. In v1, fields like
// "employeeNumber" and "manager" live at the root level. In v2 they must live
// under the enterprise schema URI key. The inner loop uses case-insensitive
// matching because v1 field names are not guaranteed to be canonically cased.
func promoteEnterpriseExtension(m map[string]interface{}) map[string]interface{} {
	enterpriseAttrs := []string{
		"employeeNumber", "costCenter", "organization",
		"division", "department", "manager",
	}

	ext := make(map[string]interface{})
	for _, attr := range enterpriseAttrs {
		// Case-insensitive scan because v1 sources may vary in casing.
		for k, v := range m {
			if strings.EqualFold(k, attr) {
				ext[attr] = v
				delete(m, k)
				break
			}
		}
	}

	if len(ext) > 0 {
		m[v2EnterpriseSchemaURI] = ext
	}
	return m
}

func flattenEnterpriseExtension(m map[string]interface{}) map[string]interface{} {
	var extKey string
	for k := range m {
		if strings.EqualFold(k, v2EnterpriseSchemaURI) {
			extKey = k
			break
		}
	}
	if extKey == "" {
		return m
	}

	ext, ok := m[extKey].(map[string]interface{})
	if !ok {
		return m
	}
	delete(m, extKey)
	for k, v := range ext {
		m[k] = v
	}
	return m
}

// fixManagerField converts the v1 "managerId" field to the v2 "value" field
// inside the enterprise extension's "manager" complex attribute. In SCIM v1 the
// reference to a manager is stored as managerId; in v2 it is stored as "value"
// (the standard field name for referenced resource IDs in complex attributes).
// The function mutates the map in-place; the key search is case-insensitive to
// tolerate varied v1 payloads.
func fixManagerField(m map[string]interface{}) {
	var extKey string
	for k := range m {
		if strings.EqualFold(k, v2EnterpriseSchemaURI) {
			extKey = k
			break
		}
	}

	var managerMap map[string]interface{}
	if extKey != "" {
		if ext, ok := m[extKey].(map[string]interface{}); ok {
			for k, v := range ext {
				if strings.EqualFold(k, "manager") {
					if mm, ok := v.(map[string]interface{}); ok {
						managerMap = mm
					}
					break
				}
			}
		}
	}

	if managerMap == nil {
		return
	}

	for k, v := range managerMap {
		if strings.EqualFold(k, "managerId") {
			managerMap["value"] = v
			delete(managerMap, k)
			break
		}
	}
}

func fixManagerFieldToV1(m map[string]interface{}) {
	for k, v := range m {
		if strings.EqualFold(k, "manager") {
			if mm, ok := v.(map[string]interface{}); ok {
				for mk, mv := range mm {
					if strings.EqualFold(mk, "value") {
						mm["managerId"] = mv
						delete(mm, mk)
						break
					}
				}
			}
			break
		}
	}
}

func fixGroupsMembership(m map[string]interface{}) {
	for k, v := range m {
		if strings.EqualFold(k, "groups") {
			if arr, ok := v.([]interface{}); ok {
				for i, elem := range arr {
					if gm, ok := elem.(map[string]interface{}); ok {
						for mk, mv := range gm {
							if strings.EqualFold(mk, "id") {
								gm["value"] = mv
								delete(gm, mk)
								break
							}
						}
						arr[i] = gm
					}
				}
				m[k] = arr
			}
			break
		}
	}
}

func toStringSlice(v interface{}) ([]string, bool) {
	switch sv := v.(type) {
	case []string:
		return sv, true
	case []interface{}:
		result := make([]string, 0, len(sv))
		for _, item := range sv {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result, true
	}
	return nil, false
}
