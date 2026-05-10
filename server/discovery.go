package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cerberauth/scimply/protocol"
	"github.com/cerberauth/scimply/schema"
)

const (
	schemaServiceProviderConfig = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	schemaResourceType          = "urn:ietf:params:scim:schemas:core:2.0:ResourceType"
	schemaSchema                = "urn:ietf:params:scim:schemas:core:2.0:Schema"
)

func (s *Server) handleServiceProviderConfig(w http.ResponseWriter, r *http.Request) {
	spc := s.cfg.spConfig

	raw, err := json.Marshal(spc)
	if err != nil {
		protocol.NewSCIMError(http.StatusInternalServerError, "", "failed to serialize ServiceProviderConfig").Write(w)
		return
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		protocol.NewSCIMError(http.StatusInternalServerError, "", "failed to serialize ServiceProviderConfig").Write(w)
		return
	}
	m["schemas"] = []string{schemaServiceProviderConfig}

	protocol.WriteResource(w, r, m, "", "")
}

func (s *Server) handleResourceTypes(w http.ResponseWriter, r *http.Request) {

	path := r.URL.Path
	prefix := s.cfg.basePath + "/ResourceTypes"
	suffix := strings.TrimPrefix(path, prefix)
	suffix = strings.TrimPrefix(suffix, "/")

	if suffix != "" {

		rt, ok := s.cfg.registry.ResourceTypeByName(suffix)
		if !ok {
			protocol.NewSCIMError(http.StatusNotFound, "", "resource type not found: "+suffix).Write(w)
			return
		}
		m := resourceTypeToMap(rt)
		location := scheme(r) + "://" + r.Host + s.cfg.basePath + "/ResourceTypes/" + rt.Name
		protocol.WriteResource(w, r, m, location, "")
		return
	}

	rts := s.cfg.registry.ResourceTypes()
	resources := make([]interface{}, 0, len(rts))
	for _, rt := range rts {
		resources = append(resources, resourceTypeToMap(rt))
	}
	listResp := protocol.NewListResponse(len(rts), 1, len(rts), resources)
	protocol.WriteJSON(w, http.StatusOK, listResp)
}

func (s *Server) handleSchemas(w http.ResponseWriter, r *http.Request) {

	path := r.URL.Path
	prefix := s.cfg.basePath + "/Schemas"
	suffix := strings.TrimPrefix(path, prefix)
	suffix = strings.TrimPrefix(suffix, "/")

	if suffix != "" {

		sch, ok := s.cfg.registry.SchemaByID(suffix)
		if !ok {
			protocol.NewSCIMError(http.StatusNotFound, "", "schema not found: "+suffix).Write(w)
			return
		}
		m := schemaToMap(sch)
		protocol.WriteResource(w, r, m, "", "")
		return
	}

	schemas := s.cfg.registry.Schemas()
	resources := make([]interface{}, 0, len(schemas))
	for _, sch := range schemas {
		resources = append(resources, schemaToMap(sch))
	}
	listResp := protocol.NewListResponse(len(schemas), 1, len(schemas), resources)
	protocol.WriteJSON(w, http.StatusOK, listResp)
}

func resourceTypeToMap(rt *schema.ResourceType) map[string]interface{} {
	type schemaExtJSON struct {
		Schema   string `json:"schema"`
		Required bool   `json:"required"`
	}
	type rtJSON struct {
		ID               string          `json:"id,omitempty"`
		Name             string          `json:"name"`
		Description      string          `json:"description,omitempty"`
		Endpoint         string          `json:"endpoint"`
		Schema           string          `json:"schema"`
		SchemaExtensions []schemaExtJSON `json:"schemaExtensions,omitempty"`
	}
	exts := make([]schemaExtJSON, 0, len(rt.SchemaExtensions))
	for _, e := range rt.SchemaExtensions {
		exts = append(exts, schemaExtJSON{Schema: e.Schema, Required: e.Required})
	}
	obj := rtJSON{
		ID:               rt.ID,
		Name:             rt.Name,
		Description:      rt.Description,
		Endpoint:         rt.Endpoint,
		Schema:           rt.Schema,
		SchemaExtensions: exts,
	}
	raw, _ := json.Marshal(obj)
	var m map[string]interface{}
	_ = json.Unmarshal(raw, &m)
	if m == nil {
		m = make(map[string]interface{})
	}
	m["schemas"] = []string{schemaResourceType}
	return m
}

func schemaToMap(sch *schema.Schema) map[string]interface{} {
	m := map[string]interface{}{
		"schemas":     []string{schemaSchema},
		"id":          sch.ID,
		"name":        sch.Name,
		"description": sch.Description,
		"attributes":  attributesToSlice(sch.Attributes),
	}
	return m
}

func attributesToSlice(attrs []schema.Attribute) []interface{} {
	out := make([]interface{}, 0, len(attrs))
	for _, a := range attrs {
		m := map[string]interface{}{
			"name":        a.Name,
			"type":        string(a.Type),
			"multiValued": a.MultiValued,
			"description": a.Description,
			"required":    a.Required,
			"caseExact":   a.CaseExact,
			"mutability":  string(a.Mutability),
			"returned":    string(a.Returned),
			"uniqueness":  string(a.Uniqueness),
		}
		if len(a.CanonicalValues) > 0 {
			m["canonicalValues"] = a.CanonicalValues
		}
		if len(a.ReferenceTypes) > 0 {
			m["referenceTypes"] = a.ReferenceTypes
		}
		if len(a.SubAttributes) > 0 {
			m["subAttributes"] = attributesToSlice(a.SubAttributes)
		}
		out = append(out, m)
	}
	return out
}

func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
