package schema

import "testing"

func TestTypeConstants(t *testing.T) {
	tests := []struct {
		name string
		got  Type
		want string
	}{
		{"TypeString", TypeString, "string"},
		{"TypeBoolean", TypeBoolean, "boolean"},
		{"TypeDecimal", TypeDecimal, "decimal"},
		{"TypeInteger", TypeInteger, "integer"},
		{"TypeDateTime", TypeDateTime, "dateTime"},
		{"TypeBinary", TypeBinary, "binary"},
		{"TypeReference", TypeReference, "reference"},
		{"TypeComplex", TypeComplex, "complex"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestMutabilityConstants(t *testing.T) {
	tests := []struct {
		name string
		got  Mutability
		want string
	}{
		{"ReadOnly", MutabilityReadOnly, "readOnly"},
		{"ReadWrite", MutabilityReadWrite, "readWrite"},
		{"Immutable", MutabilityImmutable, "immutable"},
		{"WriteOnly", MutabilityWriteOnly, "writeOnly"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestReturnedConstants(t *testing.T) {
	tests := []struct {
		name string
		got  Returned
		want string
	}{
		{"Always", ReturnedAlways, "always"},
		{"Never", ReturnedNever, "never"},
		{"Default", ReturnedDefault, "default"},
		{"Request", ReturnedRequest, "request"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestUniquenessConstants(t *testing.T) {
	tests := []struct {
		name string
		got  Uniqueness
		want string
	}{
		{"None", UniquenessNone, "none"},
		{"Server", UniquenessServer, "server"},
		{"Global", UniquenessGlobal, "global"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestAttributeZeroValue(t *testing.T) {
	var a Attribute
	if a.Name != "" {
		t.Errorf("expected empty Name, got %q", a.Name)
	}
	if a.MultiValued != false {
		t.Error("expected MultiValued to default to false")
	}
	if a.Required != false {
		t.Error("expected Required to default to false")
	}
	if len(a.SubAttributes) != 0 {
		t.Error("expected SubAttributes to default to nil/empty")
	}
	if len(a.ReferenceTypes) != 0 {
		t.Error("expected ReferenceTypes to default to nil/empty")
	}
}
