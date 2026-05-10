package store

import (
	"context"

	"github.com/cerberauth/scimply/resource"
)

type SortOrder int

const (
	SortAscending SortOrder = iota
	SortDescending
)

type ListParams struct {
	Filter             resource.FilterExpression
	SortBy             string
	SortOrder          SortOrder
	StartIndex         int
	Count              int
	Attributes         []string
	ExcludedAttributes []string
}

type ListResult struct {
	Resources         []*resource.Resource
	TotalResults      int
	StartIndex        int
	ItemsPerPage      int
	NeedsClientFilter bool
}

type ResourceStore interface {
	Create(ctx context.Context, resourceType string, res *resource.Resource) (*resource.Resource, error)

	Get(ctx context.Context, resourceType string, id string) (*resource.Resource, error)

	List(ctx context.Context, resourceType string, params ListParams) (*ListResult, error)

	Replace(ctx context.Context, resourceType string, id string, res *resource.Resource) (*resource.Resource, error)

	Patch(ctx context.Context, resourceType string, id string, ops []resource.PatchOp) (*resource.Resource, error)

	Delete(ctx context.Context, resourceType string, id string) error
}
