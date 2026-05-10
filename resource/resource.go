package resource

import (
	"encoding/json"
	"strings"
	"time"
)

type Meta struct {
	ResourceType string    `json:"resourceType,omitempty"`
	Created      time.Time `json:"created,omitempty"`
	LastModified time.Time `json:"lastModified,omitempty"`
	Location     string    `json:"location,omitempty"`
	Version      string    `json:"version,omitempty"`
}

type Resource struct {
	Schemas    []string               `json:"schemas"`
	ID         string                 `json:"id,omitempty"`
	ExternalID string                 `json:"externalId,omitempty"`
	Meta       Meta                   `json:"meta,omitempty"`
	Attributes map[string]interface{} `json:"-"`
}

func (r *Resource) ToMap() map[string]interface{} {
	m := make(map[string]interface{})
	for k, v := range r.Attributes {
		m[k] = v
	}

	if len(r.Schemas) > 0 {
		m["schemas"] = r.Schemas
	}
	if r.ID != "" {
		m["id"] = r.ID
	}
	if r.ExternalID != "" {
		m["externalId"] = r.ExternalID
	}
	meta := r.Meta
	if !meta.Created.IsZero() || !meta.LastModified.IsZero() ||
		meta.ResourceType != "" || meta.Location != "" || meta.Version != "" {
		metaMap := map[string]interface{}{}
		if meta.ResourceType != "" {
			metaMap["resourceType"] = meta.ResourceType
		}
		if !meta.Created.IsZero() {
			metaMap["created"] = meta.Created.UTC().Format(time.RFC3339)
		}
		if !meta.LastModified.IsZero() {
			metaMap["lastModified"] = meta.LastModified.UTC().Format(time.RFC3339)
		}
		if meta.Location != "" {
			metaMap["location"] = meta.Location
		}
		if meta.Version != "" {
			metaMap["version"] = meta.Version
		}
		m["meta"] = metaMap
	}
	return m
}

func FromMap(m map[string]interface{}) *Resource {
	r := &Resource{
		Attributes: make(map[string]interface{}),
	}
	for k, v := range m {
		switch strings.ToLower(k) {
		case "schemas":
			switch sv := v.(type) {
			case []interface{}:
				for _, s := range sv {
					if str, ok := s.(string); ok {
						r.Schemas = append(r.Schemas, str)
					}
				}
			case []string:
				r.Schemas = append(r.Schemas, sv...)
			}
		case "id":
			if str, ok := v.(string); ok {
				r.ID = str
			}
		case "externalid":
			if str, ok := v.(string); ok {
				r.ExternalID = str
			}
		case "meta":
			if mm, ok := v.(map[string]interface{}); ok {
				r.Meta = parseMeta(mm)
			}
		default:
			r.Attributes[k] = v
		}
	}
	return r
}

func parseMeta(m map[string]interface{}) Meta {
	var meta Meta
	for k, v := range m {
		switch strings.ToLower(k) {
		case "resourcetype":
			if str, ok := v.(string); ok {
				meta.ResourceType = str
			}
		case "created":
			if str, ok := v.(string); ok {
				t, err := time.Parse(time.RFC3339, str)
				if err == nil {
					meta.Created = t
				}
			}
		case "lastmodified":
			if str, ok := v.(string); ok {
				t, err := time.Parse(time.RFC3339, str)
				if err == nil {
					meta.LastModified = t
				}
			}
		case "location":
			if str, ok := v.(string); ok {
				meta.Location = str
			}
		case "version":
			if str, ok := v.(string); ok {
				meta.Version = str
			}
		}
	}
	return meta
}

func (r *Resource) Clone() *Resource {
	if r == nil {
		return nil
	}

	data, err := json.Marshal(r.ToMap())
	if err != nil {
		c := *r
		attrs := make(map[string]interface{}, len(r.Attributes))
		for k, v := range r.Attributes {
			attrs[k] = v
		}
		c.Attributes = attrs
		return &c
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		c := *r
		return &c
	}
	return FromMap(m)
}
