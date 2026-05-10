package schema

import "strings"

type Schema struct {
	ID          string
	Name        string
	Description string
	Attributes  []Attribute
}

func (s *Schema) AttributeByName(name string) *Attribute {
	for i := range s.Attributes {
		if strings.EqualFold(s.Attributes[i].Name, name) {
			return &s.Attributes[i]
		}
	}
	return nil
}
