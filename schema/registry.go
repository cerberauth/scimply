package schema

import (
	"strings"
	"sync"
)

type Registry struct {
	mu            sync.RWMutex
	schemas       map[string]*Schema
	resourceTypes map[string]*ResourceType
}

func NewRegistry() *Registry {
	return &Registry{
		schemas:       make(map[string]*Schema),
		resourceTypes: make(map[string]*ResourceType),
	}
}

func (r *Registry) RegisterSchema(s *Schema) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schemas[strings.ToLower(s.ID)] = s
}

func (r *Registry) RegisterResourceType(rt *ResourceType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resourceTypes[strings.ToLower(rt.Name)] = rt
}

func (r *Registry) SchemaByID(id string) (*Schema, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.schemas[strings.ToLower(id)]
	return s, ok
}

func (r *Registry) ResourceTypeByName(name string) (*ResourceType, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.resourceTypes[strings.ToLower(name)]
	return rt, ok
}

func (r *Registry) ResourceTypeByEndpoint(endpoint string) (*ResourceType, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, rt := range r.resourceTypes {
		if strings.EqualFold(rt.Endpoint, endpoint) {
			return rt, true
		}
	}
	return nil, false
}

func (r *Registry) Schemas() []*Schema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Schema, 0, len(r.schemas))
	for _, s := range r.schemas {
		out = append(out, s)
	}
	return out
}

func (r *Registry) ResourceTypes() []*ResourceType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*ResourceType, 0, len(r.resourceTypes))
	for _, rt := range r.resourceTypes {
		out = append(out, rt)
	}
	return out
}

func (r *Registry) RegisterDefaults() {
	r.RegisterSchema(CoreUserSchema())
	r.RegisterSchema(CoreGroupSchema())
	r.RegisterSchema(EnterpriseUserSchema())
	r.RegisterResourceType(UserResourceType())
	r.RegisterResourceType(GroupResourceType())
}
