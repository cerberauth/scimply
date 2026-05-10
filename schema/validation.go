package schema

import (
	"fmt"
	"strings"
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: field %q: %s", e.Field, e.Message)
}

func Validate(s *Schema, attrs map[string]interface{}) []ValidationError {
	return validateAttributes(s.Attributes, attrs)
}

func ValidateExtension(ext *Schema, attrs map[string]interface{}) []ValidationError {
	return validateAttributes(ext.Attributes, attrs)
}

func validateAttributes(defs []Attribute, attrs map[string]interface{}) []ValidationError {
	var errs []ValidationError

	for i := range defs {
		def := &defs[i]
		val, present := lookupAttr(attrs, def.Name)

		if def.Required && !present {
			errs = append(errs, ValidationError{
				Field:   def.Name,
				Message: "required attribute is missing",
			})
			continue
		}

		if !present || val == nil {
			continue
		}

		typeErr := checkType(def, val)
		if typeErr != nil {
			errs = append(errs, *typeErr)
			continue
		}

		if def.Type == TypeComplex && len(def.SubAttributes) > 0 {
			if def.MultiValued {
				if slice, ok := val.([]interface{}); ok {
					for idx, item := range slice {
						if m, ok := item.(map[string]interface{}); ok {
							subErrs := validateAttributes(def.SubAttributes, m)
							for _, se := range subErrs {
								errs = append(errs, ValidationError{
									Field:   fmt.Sprintf("%s[%d].%s", def.Name, idx, se.Field),
									Message: se.Message,
								})
							}
						}
					}
				}
			} else {
				if m, ok := val.(map[string]interface{}); ok {
					subErrs := validateAttributes(def.SubAttributes, m)
					for _, se := range subErrs {
						errs = append(errs, ValidationError{
							Field:   def.Name + "." + se.Field,
							Message: se.Message,
						})
					}
				}
			}
		}
	}

	return errs
}

func lookupAttr(attrs map[string]interface{}, name string) (interface{}, bool) {
	for k, v := range attrs {
		if strings.EqualFold(k, name) {
			return v, true
		}
	}
	return nil, false
}

func checkType(def *Attribute, val interface{}) *ValidationError {
	if def.MultiValued {
		if _, ok := val.([]interface{}); !ok {
			return &ValidationError{
				Field:   def.Name,
				Message: fmt.Sprintf("expected a JSON array for multi-valued attribute, got %T", val),
			}
		}
		return nil
	}

	switch def.Type {
	case TypeString, TypeDateTime, TypeBinary, TypeReference:
		if _, ok := val.(string); !ok {
			return &ValidationError{
				Field:   def.Name,
				Message: fmt.Sprintf("expected string, got %T", val),
			}
		}
	case TypeBoolean:
		if _, ok := val.(bool); !ok {
			return &ValidationError{
				Field:   def.Name,
				Message: fmt.Sprintf("expected boolean, got %T", val),
			}
		}
	case TypeInteger:
		switch val.(type) {
		case int, int8, int16, int32, int64,
			uint, uint8, uint16, uint32, uint64,
			float64:

		default:
			return &ValidationError{
				Field:   def.Name,
				Message: fmt.Sprintf("expected integer, got %T", val),
			}
		}
	case TypeDecimal:
		switch val.(type) {
		case float32, float64,
			int, int8, int16, int32, int64,
			uint, uint8, uint16, uint32, uint64:

		default:
			return &ValidationError{
				Field:   def.Name,
				Message: fmt.Sprintf("expected decimal, got %T", val),
			}
		}
	case TypeComplex:
		if _, ok := val.(map[string]interface{}); !ok {
			return &ValidationError{
				Field:   def.Name,
				Message: fmt.Sprintf("expected object for complex attribute, got %T", val),
			}
		}
	}

	return nil
}
