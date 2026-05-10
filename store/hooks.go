package store

import (
	"context"

	"github.com/cerberauth/scimply/resource"
)

type PreCreateHook func(ctx context.Context, resourceType string, res *resource.Resource) error

type PostCreateHook func(ctx context.Context, resourceType string, res *resource.Resource)

type PreUpdateHook func(ctx context.Context, resourceType string, id string, res *resource.Resource) error

type PostUpdateHook func(ctx context.Context, resourceType string, res *resource.Resource)

type PreDeleteHook func(ctx context.Context, resourceType string, id string) error

type PostDeleteHook func(ctx context.Context, resourceType string, id string)

type Hooks struct {
	PreCreate  []PreCreateHook
	PostCreate []PostCreateHook
	PreUpdate  []PreUpdateHook
	PostUpdate []PostUpdateHook
	PreDelete  []PreDeleteHook
	PostDelete []PostDeleteHook
}
