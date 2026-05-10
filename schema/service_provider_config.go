package schema

type Supported struct {
	Supported bool `json:"supported"`
}

type BulkConfig struct {
	Supported      bool `json:"supported"`
	MaxOperations  int  `json:"maxOperations"`
	MaxPayloadSize int  `json:"maxPayloadSize"`
}

type FilterConfig struct {
	Supported  bool `json:"supported"`
	MaxResults int  `json:"maxResults"`
}

type AuthenticationScheme struct {
	Type             string `json:"type"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	SpecURI          string `json:"specUri,omitempty"`
	DocumentationURI string `json:"documentationUri,omitempty"`
	Primary          bool   `json:"primary,omitempty"`
}

type ServiceProviderConfig struct {
	DocumentationURI      string                 `json:"documentationUri,omitempty"`
	Patch                 Supported              `json:"patch"`
	Bulk                  BulkConfig             `json:"bulk"`
	Filter                FilterConfig           `json:"filter"`
	ChangePassword        Supported              `json:"changePassword"`
	Sort                  Supported              `json:"sort"`
	ETag                  Supported              `json:"etag"`
	AuthenticationSchemes []AuthenticationScheme `json:"authenticationSchemes"`
}
