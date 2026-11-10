package server

import (
	"context"
	"strings"

	"github.com/cerberauth/scimply/resource"
	"github.com/cerberauth/scimply/schema"
)

const passwordAttr = "password"

// resourceToMap serializes a resource for an HTTP response. Every resource
// returned to a client (single resource, list, .search and bulk) must go
// through this function so that schema-level response rules are applied.
//
// Attributes defined with returned "never" are removed (RFC 7643 §2.4, §7).
func (s *Server) resourceToMap(resourceType string, res *resource.Resource) map[string]interface{} {
	m := res.ToMap()

	rt, ok := s.cfg.registry.ResourceTypeByName(resourceType)
	if !ok {
		return m
	}

	if core, ok := s.cfg.registry.SchemaByID(rt.Schema); ok {
		stripNeverReturned(core.Attributes, m)
	}
	for _, ext := range rt.SchemaExtensions {
		extSchema, ok := s.cfg.registry.SchemaByID(ext.Schema)
		if !ok {
			continue
		}
		for k, v := range m {
			if !strings.EqualFold(k, ext.Schema) {
				continue
			}
			if extMap, ok := v.(map[string]interface{}); ok {
				stripNeverReturned(extSchema.Attributes, extMap)
			}
		}
	}
	return m
}

func stripNeverReturned(defs []schema.Attribute, m map[string]interface{}) {
	for i := range defs {
		def := &defs[i]
		for k, v := range m {
			if !strings.EqualFold(k, def.Name) {
				continue
			}
			if def.Returned == schema.ReturnedNever {
				delete(m, k)
				continue
			}
			if def.Type != schema.TypeComplex || len(def.SubAttributes) == 0 {
				continue
			}
			switch val := v.(type) {
			case map[string]interface{}:
				stripNeverReturned(def.SubAttributes, val)
			case []interface{}:
				for _, item := range val {
					if im, ok := item.(map[string]interface{}); ok {
						stripNeverReturned(def.SubAttributes, im)
					}
				}
			}
		}
	}
}

// hashPasswordAttr replaces the top-level password attribute of a resource
// with its hashed value when a password hasher is configured.
func (s *Server) hashPasswordAttr(ctx context.Context, attrs map[string]interface{}) error {
	if s.cfg.passwordHasher == nil {
		return nil
	}
	for k, v := range attrs {
		if !strings.EqualFold(k, passwordAttr) {
			continue
		}
		plain, ok := v.(string)
		if !ok || plain == "" {
			continue
		}
		hashed, err := s.cfg.passwordHasher(ctx, plain)
		if err != nil {
			return err
		}
		attrs[k] = hashed
	}
	return nil
}

// hashPasswordPatchOps hashes password values carried by PATCH operations,
// either targeted by path ("password") or embedded in a path-less value.
func (s *Server) hashPasswordPatchOps(ctx context.Context, ops []resource.PatchOp) error {
	if s.cfg.passwordHasher == nil {
		return nil
	}
	for i := range ops {
		op := &ops[i]
		if op.Op == resource.PatchOpRemove {
			continue
		}
		if op.Path == nil {
			if vm, ok := op.Value.(map[string]interface{}); ok {
				if err := s.hashPasswordAttr(ctx, vm); err != nil {
					return err
				}
			}
			continue
		}
		attr := op.Path.Attribute
		if attr.SubAttribute != "" || op.Path.SubAttribute != "" || !strings.EqualFold(attr.AttributeName, passwordAttr) {
			continue
		}
		if attr.Schema != "" && !strings.EqualFold(attr.Schema, schema.UserSchemaURI) {
			continue
		}
		if plain, ok := op.Value.(string); ok && plain != "" {
			hashed, err := s.cfg.passwordHasher(ctx, plain)
			if err != nil {
				return err
			}
			op.Value = hashed
		}
	}
	return nil
}
