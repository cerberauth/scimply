package schema

type SchemaExtension struct {
	Schema   string
	Required bool
}

type ResourceType struct {
	ID               string
	Name             string
	Description      string
	Endpoint         string
	Schema           string
	SchemaExtensions []SchemaExtension
}
