package schema

import "testing"

func validationSchema() *Schema {
	return &Schema{
		ID:   "urn:test:validation",
		Name: "TestResource",
		Attributes: []Attribute{
			{
				Name:       "id",
				Type:       TypeString,
				Required:   false,
				Mutability: MutabilityReadOnly,
				Returned:   ReturnedAlways,
			},
			{
				Name:       "userName",
				Type:       TypeString,
				Required:   true,
				Mutability: MutabilityReadWrite,
				Returned:   ReturnedDefault,
			},
			{
				Name:       "active",
				Type:       TypeBoolean,
				Required:   false,
				Mutability: MutabilityReadWrite,
				Returned:   ReturnedDefault,
			},
			{
				Name:        "emails",
				Type:        TypeComplex,
				MultiValued: true,
				Required:    false,
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				SubAttributes: []Attribute{
					{Name: "value", Type: TypeString, Required: true},
					{Name: "primary", Type: TypeBoolean},
				},
			},
		},
	}
}

func errCount(errs []ValidationError, field string) int {
	n := 0
	for _, e := range errs {
		if e.Field == field {
			n++
		}
	}
	return n
}

func TestValidate_MissingRequiredField(t *testing.T) {
	s := validationSchema()
	attrs := map[string]interface{}{

		"active": true,
	}
	errs := Validate(s, attrs)
	if len(errs) == 0 {
		t.Fatal("expected validation errors for missing required field, got none")
	}
	if errCount(errs, "userName") == 0 {
		t.Errorf("expected error for field 'userName', got: %v", errs)
	}
}

func TestValidate_RequiredFieldPresent(t *testing.T) {
	s := validationSchema()
	attrs := map[string]interface{}{
		"userName": "jdoe",
		"active":   true,
	}
	errs := Validate(s, attrs)
	if len(errs) != 0 {
		t.Errorf("expected no validation errors, got: %v", errs)
	}
}

func TestValidate_UnknownAttributesTolerated(t *testing.T) {
	s := validationSchema()
	attrs := map[string]interface{}{
		"userName":      "jdoe",
		"customField1":  "value",
		"anotherCustom": 42,
	}
	errs := Validate(s, attrs)
	if len(errs) != 0 {
		t.Errorf("expected no errors for unknown attributes, got: %v", errs)
	}
}

func TestValidate_MultipleErrorsCollected(t *testing.T) {
	s := &Schema{
		ID: "urn:test:multi",
		Attributes: []Attribute{
			{Name: "field1", Type: TypeString, Required: true},
			{Name: "field2", Type: TypeString, Required: true},
			{Name: "field3", Type: TypeString, Required: true},
		},
	}
	errs := Validate(s, map[string]interface{}{})
	if len(errs) != 3 {
		t.Errorf("expected 3 errors for 3 missing required fields, got %d: %v", len(errs), errs)
	}
}

func TestValidate_TypeMismatch_String(t *testing.T) {
	s := validationSchema()
	attrs := map[string]interface{}{
		"userName": 12345,
	}
	errs := Validate(s, attrs)
	if errCount(errs, "userName") == 0 {
		t.Errorf("expected type error for userName, got: %v", errs)
	}
}

func TestValidate_TypeMismatch_Boolean(t *testing.T) {
	s := validationSchema()
	attrs := map[string]interface{}{
		"userName": "jdoe",
		"active":   "yes",
	}
	errs := Validate(s, attrs)
	if errCount(errs, "active") == 0 {
		t.Errorf("expected type error for 'active', got: %v", errs)
	}
}

func TestValidate_MultiValuedNotSlice(t *testing.T) {
	s := validationSchema()
	attrs := map[string]interface{}{
		"userName": "jdoe",
		"emails":   "notaslice",
	}
	errs := Validate(s, attrs)
	if errCount(errs, "emails") == 0 {
		t.Errorf("expected type error for multi-valued 'emails', got: %v", errs)
	}
}

func TestValidate_ComplexSubAttrRequired(t *testing.T) {
	s := validationSchema()
	attrs := map[string]interface{}{
		"userName": "jdoe",
		"emails": []interface{}{
			map[string]interface{}{

				"primary": true,
			},
		},
	}
	errs := Validate(s, attrs)

	found := false
	for _, e := range errs {
		if e.Field == "emails[0].value" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error for emails[0].value, got: %v", errs)
	}
}

func TestValidate_NilValueForOptionalField(t *testing.T) {
	s := validationSchema()
	attrs := map[string]interface{}{
		"userName": "jdoe",
		"active":   nil,
	}
	errs := Validate(s, attrs)
	if len(errs) != 0 {
		t.Errorf("expected no errors for nil optional field, got: %v", errs)
	}
}

func TestValidationError_Error(t *testing.T) {
	e := &ValidationError{Field: "userName", Message: "required attribute is missing"}
	got := e.Error()
	want := `validation error: field "userName": required attribute is missing`
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestValidateExtension_MissingRequired(t *testing.T) {
	ext := &Schema{
		ID: EnterpriseUserSchemaURI,
		Attributes: []Attribute{
			{Name: "employeeNumber", Type: TypeString, Required: true},
		},
	}
	errs := ValidateExtension(ext, map[string]interface{}{})
	if len(errs) == 0 {
		t.Fatal("expected validation errors for extension, got none")
	}
	if errCount(errs, "employeeNumber") == 0 {
		t.Errorf("expected error for 'employeeNumber', got: %v", errs)
	}
}

func TestValidateExtension_Valid(t *testing.T) {
	ext := &Schema{
		ID: EnterpriseUserSchemaURI,
		Attributes: []Attribute{
			{Name: "employeeNumber", Type: TypeString, Required: true},
		},
	}
	errs := ValidateExtension(ext, map[string]interface{}{
		"employeeNumber": "EMP-001",
	})
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid extension attrs, got: %v", errs)
	}
}

func TestValidate_CaseInsensitiveAttrLookup(t *testing.T) {
	s := validationSchema()

	attrs := map[string]interface{}{
		"USERNAME": "jdoe",
	}
	errs := Validate(s, attrs)
	if len(errs) != 0 {
		t.Errorf("expected no errors with case-insensitive attr key, got: %v", errs)
	}
}
