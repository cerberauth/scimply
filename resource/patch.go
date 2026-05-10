package resource

import (
	"errors"
	"fmt"
	"strings"
)

var ErrBadPatch = errors.New("scimply: invalid patch operation")

var ErrNoTarget = errors.New("scimply: no target for patch operation")

type PatchOpType string

const (
	PatchOpAdd     PatchOpType = "add"
	PatchOpRemove  PatchOpType = "remove"
	PatchOpReplace PatchOpType = "replace"
)

type PatchOp struct {
	Op    PatchOpType
	Path  *PatchPath
	Value interface{}
}

type PatchRequest struct {
	Schemas    []string
	Operations []PatchOp
}

func ParsePatchRequest(body map[string]interface{}) (*PatchRequest, error) {
	req := &PatchRequest{}

	if s, ok := body[schemasKey]; ok {
		switch sv := s.(type) {
		case []interface{}:
			for _, v := range sv {
				if str, ok := v.(string); ok {
					req.Schemas = append(req.Schemas, str)
				}
			}
		case []string:
			req.Schemas = sv
		}
	}

	opsRaw, ok := body["Operations"]
	if !ok {
		for k, v := range body {
			if strings.EqualFold(k, "operations") {
				opsRaw = v
				ok = true
				break
			}
		}
	}
	if !ok {
		return nil, ErrBadPatch
	}

	ops, ok := opsRaw.([]interface{})
	if !ok {
		return nil, ErrBadPatch
	}

	for i, opRaw := range ops {
		opMap, ok := opRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%w: operation %d is not an object", ErrBadPatch, i)
		}
		op, err := parsePatchOp(opMap)
		if err != nil {
			return nil, fmt.Errorf("%w: operation %d: %v", ErrBadPatch, i, err)
		}
		req.Operations = append(req.Operations, op)
	}

	return req, nil
}

func parsePatchOp(m map[string]interface{}) (PatchOp, error) {
	op := PatchOp{}

	opStr, ok := lookupMapString(m, "op")
	if !ok {
		return op, fmt.Errorf("missing 'op' field")
	}
	switch strings.ToLower(opStr) {
	case "add":
		op.Op = PatchOpAdd
	case "remove":
		op.Op = PatchOpRemove
	case "replace":
		op.Op = PatchOpReplace
	default:
		return op, fmt.Errorf("unknown op: %q", opStr)
	}

	if pathStr, ok := lookupMapString(m, "path"); ok && pathStr != "" {
		pp, err := ParsePatchPath(pathStr)
		if err != nil {
			return op, fmt.Errorf("invalid path %q: %w", pathStr, err)
		}
		op.Path = pp
	}

	if v, ok := m["value"]; ok {
		op.Value = v
	} else {
		for k, v := range m {
			if strings.EqualFold(k, "value") {
				op.Value = v
				break
			}
		}
	}

	return op, nil
}

func lookupMapString(m map[string]interface{}, key string) (string, bool) {
	lower := strings.ToLower(key)
	for k, v := range m {
		if strings.ToLower(k) == lower {
			if str, ok := v.(string); ok {
				return str, true
			}
			return "", false
		}
	}
	return "", false
}

// ApplyPatch applies a sequence of PATCH operations to a resource and returns
// the modified result. Operations are applied to a clone of the resource so
// the original is never mutated. If any operation fails the entire patch is
// aborted and the original resource is effectively unchanged (the clone is
// discarded).
func ApplyPatch(r *Resource, ops []PatchOp) (*Resource, error) {
	working := r.Clone()

	for _, op := range ops {
		if err := applyOp(working, op); err != nil {
			return nil, err
		}
	}

	return working, nil
}

func applyOp(r *Resource, op PatchOp) error {
	switch op.Op {
	case PatchOpAdd:
		return applyAdd(r, op)
	case PatchOpRemove:
		return applyRemove(r, op)
	case PatchOpReplace:
		return applyReplace(r, op)
	default:
		return fmt.Errorf("%w: unknown op %q", ErrBadPatch, op.Op)
	}
}

// applyAdd implements the SCIM "add" PATCH operation. The behaviour varies by
// how the path is specified:
//
//   - No path: value must be an object; each key is merged into the resource.
//   - Simple path (no filter, no sub-attr): if the attribute already holds an
//     array, the new value(s) are appended; otherwise the value is set directly.
//   - Path with value filter (e.g. emails[type eq "work"]): each matching array
//     element is updated in-place. If a sub-attribute is given, only that field
//     inside the matching element is updated.
//   - Path with sub-attribute only (e.g. name.givenName): the sub-attribute is
//     set on every element of the array, or on the single complex object, or a
//     new object is created if the attribute did not exist.
func applyAdd(r *Resource, op PatchOp) error {
	if op.Path == nil {
		m, ok := op.Value.(map[string]interface{})
		if !ok {
			return fmt.Errorf("%w: add without path requires object value", ErrBadPatch)
		}
		for k, v := range m {
			setAttr(r.Attributes, k, v)
		}
		return nil
	}

	pp := op.Path

	if pp.ValueFilter == nil && pp.SubAttribute == "" {
		path := pp.Attribute
		existing, _ := Get(r, path)
		if existing != nil {
			// For existing arrays, append rather than replace.
			if arr, ok := toSlice(existing); ok {
				if newArr, ok := toSlice(op.Value); ok {
					arr = append(arr, newArr...)
				} else {
					arr = append(arr, op.Value)
				}
				Set(r, path, arr)
				return nil
			}
		}
		Set(r, path, op.Value)
		return nil
	}

	if pp.ValueFilter != nil {
		// Apply the value to all array elements that match the filter.
		arr, err := getMultiValued(r, pp.Attribute)
		if err != nil {
			return err
		}
		for i, elem := range arr {
			if m, ok := elem.(map[string]interface{}); ok {
				if evalExpr(pp.ValueFilter, m) {
					if pp.SubAttribute != "" {
						setAttr(m, pp.SubAttribute, op.Value)
						arr[i] = m
					} else {
						if vm, ok := op.Value.(map[string]interface{}); ok {
							for k, v := range vm {
								setAttr(m, k, v)
							}
							arr[i] = m
						} else {
							arr[i] = op.Value
						}
					}
				}
			}
		}
		Set(r, pp.Attribute, arr)
		return nil
	}

	if pp.SubAttribute != "" {
		existing, _ := Get(r, pp.Attribute)
		if existing != nil {
			if arr, ok := toSlice(existing); ok {
				for i, elem := range arr {
					if m, ok := elem.(map[string]interface{}); ok {
						setAttr(m, pp.SubAttribute, op.Value)
						arr[i] = m
					}
				}
				Set(r, pp.Attribute, arr)
				return nil
			}
			if m, ok := existing.(map[string]interface{}); ok {
				setAttr(m, pp.SubAttribute, op.Value)
				Set(r, pp.Attribute, m)
				return nil
			}
		}
		newMap := map[string]interface{}{pp.SubAttribute: op.Value}
		Set(r, pp.Attribute, newMap)
	}
	return nil
}

// applyRemove implements the SCIM "remove" PATCH operation. Three cases:
//
//   - No filter, no sub-attribute: delete the whole attribute.
//   - No filter, with sub-attribute: delete the sub-attribute from every
//     element (array) or from the single complex object.
//   - With value filter: if no sub-attribute is given, remove matching elements
//     from the array entirely. If a sub-attribute is given, delete only that
//     field from each matching element while keeping the element itself.
//
// The in-place array filter `arr[:0]` reuses the underlying array's memory
// without allocating a new slice.
func applyRemove(r *Resource, op PatchOp) error {
	if op.Path == nil {
		return ErrNoTarget
	}

	pp := op.Path

	if pp.ValueFilter == nil && pp.SubAttribute == "" {
		Delete(r, pp.Attribute)
		return nil
	}

	if pp.ValueFilter == nil && pp.SubAttribute != "" {
		existing, _ := Get(r, pp.Attribute)
		if existing == nil {
			return nil
		}
		if arr, ok := toSlice(existing); ok {
			for i, elem := range arr {
				if m, ok := elem.(map[string]interface{}); ok {
					deleteAttr(m, pp.SubAttribute)
					arr[i] = m
				}
			}
			Set(r, pp.Attribute, arr)
			return nil
		}
		if m, ok := existing.(map[string]interface{}); ok {
			deleteAttr(m, pp.SubAttribute)
			Set(r, pp.Attribute, m)
		}
		return nil
	}

	arr, err := getMultiValued(r, pp.Attribute)
	if err != nil {
		return nil
	}

	if pp.SubAttribute == "" {
		// Keep only elements that do NOT match the filter.
		filtered := arr[:0]
		for _, elem := range arr {
			if m, ok := elem.(map[string]interface{}); ok {
				if !evalExpr(pp.ValueFilter, m) {
					filtered = append(filtered, elem)
				}
			} else {
				filtered = append(filtered, elem)
			}
		}
		Set(r, pp.Attribute, filtered)
	} else {
		// Delete only the sub-attribute from matching elements.
		for i, elem := range arr {
			if m, ok := elem.(map[string]interface{}); ok {
				if evalExpr(pp.ValueFilter, m) {
					deleteAttr(m, pp.SubAttribute)
					arr[i] = m
				}
			}
		}
		Set(r, pp.Attribute, arr)
	}
	return nil
}

func applyReplace(r *Resource, op PatchOp) error {
	if op.Path == nil {
		m, ok := op.Value.(map[string]interface{})
		if !ok {
			return fmt.Errorf("%w: replace without path requires object value", ErrBadPatch)
		}
		for k, v := range m {
			setAttr(r.Attributes, k, v)
		}
		return nil
	}

	pp := op.Path

	if pp.ValueFilter == nil && pp.SubAttribute == "" {
		Set(r, pp.Attribute, op.Value)
		return nil
	}

	if pp.ValueFilter != nil {
		arr, err := getMultiValued(r, pp.Attribute)
		if err != nil {
			Set(r, pp.Attribute, op.Value)
			return nil
		}
		for i, elem := range arr {
			if m, ok := elem.(map[string]interface{}); ok {
				if evalExpr(pp.ValueFilter, m) {
					if pp.SubAttribute != "" {
						setAttr(m, pp.SubAttribute, op.Value)
						arr[i] = m
					} else {
						if vm, ok := op.Value.(map[string]interface{}); ok {
							for k, v := range vm {
								setAttr(m, k, v)
							}
							arr[i] = m
						} else {
							arr[i] = op.Value
						}
					}
				}
			}
		}
		Set(r, pp.Attribute, arr)
		return nil
	}

	if pp.SubAttribute != "" {
		existing, _ := Get(r, pp.Attribute)
		if existing != nil {
			if arr, ok := toSlice(existing); ok {
				for i, elem := range arr {
					if m, ok := elem.(map[string]interface{}); ok {
						setAttr(m, pp.SubAttribute, op.Value)
						arr[i] = m
					}
				}
				Set(r, pp.Attribute, arr)
				return nil
			}
			if m, ok := existing.(map[string]interface{}); ok {
				setAttr(m, pp.SubAttribute, op.Value)
				Set(r, pp.Attribute, m)
				return nil
			}
		}
		newMap := map[string]interface{}{pp.SubAttribute: op.Value}
		Set(r, pp.Attribute, newMap)
	}
	return nil
}

// getMultiValued retrieves an attribute as a slice, always returning a copy.
// If the attribute holds a single scalar value rather than an array, it is
// wrapped in a one-element slice so callers can iterate uniformly. The copy
// ensures callers can mutate elements without affecting the stored resource.
func getMultiValued(r *Resource, path AttributePath) ([]interface{}, error) {
	val, ok := Get(r, path)
	if !ok || val == nil {
		return nil, ErrNoTarget
	}
	arr, ok := toSlice(val)
	if !ok {
		return []interface{}{val}, nil
	}
	result := make([]interface{}, len(arr))
	copy(result, arr)
	return result, nil
}
