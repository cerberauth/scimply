package sql

import "strings"

// ColumnRef identifies a column in a (possibly different) table.
type ColumnRef struct {
	Table  string // empty means the resource's primary table
	Column string
}

// Mapper translates SCIM attribute paths to database column references.
type Mapper struct {
	// CustomMappings maps SCIM attribute paths to column names in the primary table.
	// Kept for backwards compatibility; ColumnRefs takes precedence when both are set.
	CustomMappings map[string]string
	// ColumnRefs maps SCIM attribute paths to full column references, optionally
	// including a table override for cross-table field mappings.
	ColumnRefs map[string]ColumnRef
}

func NewMapper() *Mapper {
	return &Mapper{
		CustomMappings: make(map[string]string),
		ColumnRefs:     make(map[string]ColumnRef),
	}
}

// Ref returns the ColumnRef for a SCIM attribute path.
// Resolution order: ColumnRefs → CustomMappings → camelToSnake default.
// Returns (ColumnRef{}, false) for schema extension URIs.
func (m *Mapper) Ref(attrPath string) (ColumnRef, bool) {
	if IsExtension(attrPath) {
		return ColumnRef{}, false
	}
	if m.ColumnRefs != nil {
		if ref, ok := m.ColumnRefs[attrPath]; ok {
			return ref, true
		}
	}
	if m.CustomMappings != nil {
		if col, ok := m.CustomMappings[attrPath]; ok {
			return ColumnRef{Column: col}, true
		}
	}
	parts := strings.SplitN(attrPath, ".", 2)
	switch len(parts) {
	case 1:
		return ColumnRef{Column: camelToSnake(parts[0])}, true
	case 2:
		return ColumnRef{Column: camelToSnake(parts[0]) + "_" + camelToSnake(parts[1])}, true
	}
	return ColumnRef{Column: camelToSnake(attrPath)}, true
}

// ColumnName returns just the column name for the given SCIM attribute path.
// Use Ref instead when you need the full table-qualified reference.
func (m *Mapper) ColumnName(attrPath string) (string, bool) {
	ref, ok := m.Ref(attrPath)
	return ref.Column, ok
}

func IsExtension(attrPath string) bool {
	return strings.Contains(attrPath, ":")
}

func camelToSnake(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	runes := []rune(s)
	for i, r := range runes {
		if isUpper(r) {
			if i > 0 && runes[i-1] != '_' {
				b.WriteRune('_')
			}
			b.WriteRune(toLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isUpper(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

func toLower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}
