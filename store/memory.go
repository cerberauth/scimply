package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cerberauth/scimply/resource"
)

// MemoryStore is a thread-safe in-memory SCIM resource store. Resources are
// organised in a two-level map: resourceType → id → Resource. A sync.RWMutex
// guards the outer map; read operations hold an RLock while writes hold a full
// Lock so that concurrent reads are not serialised unnecessarily.
type MemoryStore struct {
	mu        sync.RWMutex
	resources map[string]map[string]*resource.Resource
	hooks     Hooks
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		resources: make(map[string]map[string]*resource.Resource),
	}
}

func (s *MemoryStore) WithHooks(h Hooks) *MemoryStore {
	s.hooks = h
	return s
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("%w: failed to generate id: %v", ErrInternal, err)
	}
	return hex.EncodeToString(b), nil
}

func checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", ErrInternal, ctx.Err())
	default:
		return nil
	}
}

func (s *MemoryStore) typeMap(resourceType string) map[string]*resource.Resource {
	m, ok := s.resources[resourceType]
	if !ok {
		m = make(map[string]*resource.Resource)
		s.resources[resourceType] = m
	}
	return m
}

func (s *MemoryStore) Create(ctx context.Context, resourceType string, res *resource.Resource) (*resource.Resource, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	for _, h := range s.hooks.PreCreate {
		if err := h(ctx, resourceType, res); err != nil {
			return nil, err
		}
	}

	id, err := generateID()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	clone := res.Clone()
	clone.ID = id
	clone.Meta.ResourceType = resourceType
	clone.Meta.Created = now
	clone.Meta.LastModified = now

	s.mu.Lock()
	defer s.mu.Unlock()

	m := s.typeMap(resourceType)

	if strings.EqualFold(resourceType, "User") {
		newUserName := getUserName(clone)
		if newUserName != "" {
			for _, existing := range m {
				if strings.EqualFold(getUserName(existing), newUserName) {
					return nil, fmt.Errorf("%w: userName %q already exists", ErrConflict, newUserName)
				}
			}
		}
	}

	m[id] = clone

	result := clone.Clone()
	for _, h := range s.hooks.PostCreate {
		h(ctx, resourceType, result)
	}

	return result, nil
}

func (s *MemoryStore) Get(ctx context.Context, resourceType string, id string) (*resource.Resource, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.resources[resourceType]
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, resourceType, id)
	}
	r, ok := m[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, resourceType, id)
	}
	return r.Clone(), nil
}

// List retrieves a filtered, sorted, and paginated page of resources.
//
// The lock is released before filtering and sorting so that other goroutines
// can read/write during the (potentially expensive) in-memory filter pass.
// Each matching resource is cloned before being returned so callers cannot
// accidentally mutate stored state.
//
// Pagination follows SCIM's 1-based StartIndex convention:
//   - StartIndex defaults to 1 if unset or < 1.
//   - Count = 0 returns no resources (RFC 7644 §3.4.2.4: only totalResults is returned).
//   - Count < 0 means "return everything" (no page size limit).
//   - start/end are clamped to [0, totalResults] to avoid out-of-range slices.
func (s *MemoryStore) List(ctx context.Context, resourceType string, params ListParams) (*ListResult, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	// Snapshot the type map under a read lock, then release before filtering.
	s.mu.RLock()
	m := s.resources[resourceType]
	all := make([]*resource.Resource, 0, len(m))
	for _, r := range m {
		all = append(all, r)
	}
	s.mu.RUnlock()

	var filtered []*resource.Resource
	for _, r := range all {
		if params.Filter == nil || resource.EvalFilter(params.Filter, r) {
			filtered = append(filtered, r.Clone())
		}
	}

	if params.SortBy != "" {
		order := resource.SortAscending
		if params.SortOrder == SortDescending {
			order = resource.SortDescending
		}
		resource.SortResources(filtered, params.SortBy, order)
	}

	totalResults := len(filtered)

	// SCIM uses 1-based index; normalise to 0-based for slice operations.
	startIndex := params.StartIndex
	if startIndex < 1 {
		startIndex = 1
	}

	start := startIndex - 1
	if start > totalResults {
		start = totalResults
	}

	pageSize := params.Count
	if pageSize < 0 {
		// Negative count is treated as "return everything".
		pageSize = totalResults
	}
	// A count of 0 is honoured as-is: RFC 7644 §3.4.2.4 states that count=0
	// means no resource results are returned (only totalResults).

	end := start + pageSize
	if end > totalResults {
		end = totalResults
	}

	page := filtered[start:end]
	itemsPerPage := len(page)

	return &ListResult{
		Resources:    page,
		TotalResults: totalResults,
		StartIndex:   startIndex,
		ItemsPerPage: itemsPerPage,
	}, nil
}

func (s *MemoryStore) Replace(ctx context.Context, resourceType string, id string, res *resource.Resource) (*resource.Resource, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	for _, h := range s.hooks.PreUpdate {
		if err := h(ctx, resourceType, id, res); err != nil {
			return nil, err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.resources[resourceType]
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, resourceType, id)
	}
	existing, ok := m[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, resourceType, id)
	}

	now := time.Now().UTC()
	clone := res.Clone()
	clone.ID = id
	clone.Meta.Created = existing.Meta.Created
	clone.Meta.ResourceType = resourceType
	clone.Meta.LastModified = now

	m[id] = clone

	result := clone.Clone()

	for _, h := range s.hooks.PostUpdate {
		h(ctx, resourceType, result)
	}

	return result, nil
}

// Patch applies incremental PATCH operations to a stored resource. The write
// lock is held for the entire operation (lookup + patch + store) to prevent a
// concurrent Replace from overwriting the same resource between steps.
//
// Pre-update hooks receive a clone of the resource before patching so they
// cannot alter the data that will actually be patched. If any hook returns an
// error, the patch is aborted before resource.ApplyPatch is called.
func (s *MemoryStore) Patch(ctx context.Context, resourceType string, id string, ops []resource.PatchOp) (*resource.Resource, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.resources[resourceType]
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, resourceType, id)
	}
	existing, ok := m[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, resourceType, id)
	}

	// Give hooks a snapshot of the pre-patch state; they must not retain this clone.
	cloneForHook := existing.Clone()
	for _, h := range s.hooks.PreUpdate {
		if err := h(ctx, resourceType, id, cloneForHook); err != nil {
			return nil, err
		}
	}

	patched, err := resource.ApplyPatch(existing, ops)
	if err != nil {
		if errors.Is(err, resource.ErrNoTarget) {
			return nil, fmt.Errorf("%w: %v", ErrNoTarget, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrBadPatch, err)
	}

	now := time.Now().UTC()
	patched.Meta.LastModified = now
	patched.Meta.ResourceType = resourceType

	m[id] = patched

	result := patched.Clone()

	for _, h := range s.hooks.PostUpdate {
		h(ctx, resourceType, result)
	}

	return result, nil
}

func (s *MemoryStore) Delete(ctx context.Context, resourceType string, id string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	for _, h := range s.hooks.PreDelete {
		if err := h(ctx, resourceType, id); err != nil {
			return err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.resources[resourceType]
	if !ok {
		return fmt.Errorf("%w: %s/%s", ErrNotFound, resourceType, id)
	}
	if _, ok := m[id]; !ok {
		return fmt.Errorf("%w: %s/%s", ErrNotFound, resourceType, id)
	}

	delete(m, id)

	for _, h := range s.hooks.PostDelete {
		h(ctx, resourceType, id)
	}

	return nil
}

func getUserName(r *resource.Resource) string {
	for k, v := range r.Attributes {
		if strings.EqualFold(k, "userName") {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}
