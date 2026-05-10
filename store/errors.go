package store

import "errors"

var (
	ErrNotFound     = errors.New("scimply: resource not found")
	ErrConflict     = errors.New("scimply: uniqueness constraint violation")
	ErrMutability   = errors.New("scimply: attribute is immutable or read-only")
	ErrInvalidValue = errors.New("scimply: invalid attribute value")
	ErrBadFilter    = errors.New("scimply: invalid filter expression")
	ErrBadPath      = errors.New("scimply: invalid attribute path")
	ErrBadPatch     = errors.New("scimply: invalid patch operation")
	ErrInternal     = errors.New("scimply: internal error")
	ErrTooMany      = errors.New("scimply: too many results / bulk operations")
	ErrNoTarget     = errors.New("scimply: no target for patch operation")
)
