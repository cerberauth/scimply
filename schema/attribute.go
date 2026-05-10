package schema

type Type string

const (
	TypeString    Type = "string"
	TypeBoolean   Type = "boolean"
	TypeDecimal   Type = "decimal"
	TypeInteger   Type = "integer"
	TypeDateTime  Type = "dateTime"
	TypeBinary    Type = "binary"
	TypeReference Type = "reference"
	TypeComplex   Type = "complex"
)

type Mutability string

const (
	MutabilityReadOnly  Mutability = "readOnly"
	MutabilityReadWrite Mutability = "readWrite"
	MutabilityImmutable Mutability = "immutable"
	MutabilityWriteOnly Mutability = "writeOnly"
)

type Returned string

const (
	ReturnedAlways  Returned = "always"
	ReturnedNever   Returned = "never"
	ReturnedDefault Returned = "default"
	ReturnedRequest Returned = "request"
)

type Uniqueness string

const (
	UniquenessNone   Uniqueness = "none"
	UniquenessServer Uniqueness = "server"
	UniquenessGlobal Uniqueness = "global"
)

type Attribute struct {
	Name            string
	Type            Type
	SubAttributes   []Attribute
	MultiValued     bool
	Description     string
	Required        bool
	CanonicalValues []string
	CaseExact       bool
	Mutability      Mutability
	Returned        Returned
	Uniqueness      Uniqueness
	ReferenceTypes  []string
}
