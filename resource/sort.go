package resource

import (
	"fmt"
	"sort"
	"strings"
)

type SortOrder int

const (
	SortAscending SortOrder = iota
	SortDescending
)

func SortResources(resources []*Resource, sortBy string, order SortOrder) {
	if len(resources) == 0 || sortBy == "" {
		return
	}

	path, err := ParsePath(sortBy)
	if err != nil {
		return
	}

	sort.SliceStable(resources, func(i, j int) bool {
		vi := sortValue(resources[i], path)
		vj := sortValue(resources[j], path)
		cmp := compareForSort(vi, vj)
		if order == SortDescending {
			return cmp > 0
		}
		return cmp < 0
	})
}

// sortValue extracts the effective sort key from a resource for the given path.
// For multi-valued attributes (arrays of complex objects), SCIM defines that the
// element with "primary: true" should be used. If no primary element exists, the
// first element is used as a fallback. For simple (non-array) attributes the
// value is returned directly.
func sortValue(r *Resource, path AttributePath) interface{} {
	if r == nil {
		return nil
	}
	val, ok := Get(r, path)
	if !ok {
		return nil
	}
	arr, ok := toSlice(val)
	if !ok {
		return val
	}
	// Prefer the primary element for multi-valued attributes.
	for _, elem := range arr {
		if m, ok := elem.(map[string]interface{}); ok {
			if isPrimary(m) {
				if path.SubAttribute != "" {
					return lookupKey(m, path.SubAttribute)
				}
				return extractSortableValue(m)
			}
		}
	}
	// No primary element found; fall back to the first element.
	if len(arr) > 0 {
		if m, ok := arr[0].(map[string]interface{}); ok {
			if path.SubAttribute != "" {
				return lookupKey(m, path.SubAttribute)
			}
			return extractSortableValue(m)
		}
		return arr[0]
	}
	return nil
}

func isPrimary(m map[string]interface{}) bool {
	v := lookupKey(m, "primary")
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func extractSortableValue(m map[string]interface{}) interface{} {
	if v := lookupKey(m, "value"); v != nil {
		return v
	}
	return fmt.Sprintf("%v", m)
}

func compareForSort(a, b interface{}) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}
	af, aIsNum := toFloat(a)
	bf, bIsNum := toFloat(b)
	if aIsNum && bIsNum {
		if af < bf {
			return -1
		}
		if af > bf {
			return 1
		}
		return 0
	}
	as := strings.ToLower(fmt.Sprintf("%v", a))
	bs := strings.ToLower(fmt.Sprintf("%v", b))
	return strings.Compare(as, bs)
}
