package resource

import "strings"

func Get(r *Resource, path AttributePath) (interface{}, bool) {
	if r == nil {
		return nil, false
	}
	if r.Attributes == nil {
		return nil, false
	}

	val := lookupAttr(r.Attributes, path.AttributeName)
	if val == nil {
		return nil, false
	}

	if path.SubAttribute == "" {
		return val, true
	}

	if m, ok := val.(map[string]interface{}); ok {
		sub := lookupAttr(m, path.SubAttribute)
		if sub == nil {
			return nil, false
		}
		return sub, true
	}
	return nil, false
}

func Set(r *Resource, path AttributePath, value interface{}) {
	if r == nil {
		return
	}
	if r.Attributes == nil {
		r.Attributes = make(map[string]interface{})
	}

	if path.SubAttribute == "" {
		setAttr(r.Attributes, path.AttributeName, value)
		return
	}

	parent := lookupAttr(r.Attributes, path.AttributeName)
	var parentMap map[string]interface{}
	if parent == nil {
		parentMap = make(map[string]interface{})
	} else if m, ok := parent.(map[string]interface{}); ok {
		parentMap = m
	} else {
		parentMap = make(map[string]interface{})
	}
	setAttr(parentMap, path.SubAttribute, value)
	setAttr(r.Attributes, path.AttributeName, parentMap)
}

func Delete(r *Resource, path AttributePath) {
	if r == nil || r.Attributes == nil {
		return
	}

	if path.SubAttribute == "" {
		deleteAttr(r.Attributes, path.AttributeName)
		return
	}

	parent := lookupAttr(r.Attributes, path.AttributeName)
	if m, ok := parent.(map[string]interface{}); ok {
		deleteAttr(m, path.SubAttribute)
	}
}

func lookupAttr(m map[string]interface{}, name string) interface{} {
	if v, ok := m[name]; ok {
		return v
	}
	lower := strings.ToLower(name)
	for k, v := range m {
		if strings.ToLower(k) == lower {
			return v
		}
	}
	return nil
}

func setAttr(m map[string]interface{}, name string, value interface{}) {

	lower := strings.ToLower(name)
	for k := range m {
		if strings.ToLower(k) == lower {
			m[k] = value
			return
		}
	}
	m[name] = value
}

func deleteAttr(m map[string]interface{}, name string) {
	lower := strings.ToLower(name)
	for k := range m {
		if strings.ToLower(k) == lower {
			delete(m, k)
			return
		}
	}
}
