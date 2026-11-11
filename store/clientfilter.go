package store

import (
	"fmt"

	"github.com/cerberauth/scimply/resource"
)

// DefaultMaxFilterScan is the default number of resources a store scans
// in-process when a filter cannot be translated to a native query.
const DefaultMaxFilterScan = 10000

// ClientFilter filters, sorts and paginates resources in-process. Stores use
// it when a filter cannot be pushed down to the backend: every candidate
// resource is passed to Add, then Result builds the requested page.
//
// Scanning is bounded: once more than maxScan resources have been added, Add
// returns ErrTooMany instead of silently truncating results.
type ClientFilter struct {
	params  ListParams
	maxScan int
	scanned int
	matched []*resource.Resource
}

// NewClientFilter returns a ClientFilter for params. A maxScan <= 0 disables
// the bound.
func NewClientFilter(params ListParams, maxScan int) *ClientFilter {
	return &ClientFilter{params: params, maxScan: maxScan}
}

// Add evaluates the filter against r and keeps it when it matches.
func (c *ClientFilter) Add(r *resource.Resource) error {
	c.scanned++
	if c.maxScan > 0 && c.scanned > c.maxScan {
		return fmt.Errorf("%w: filter cannot be evaluated by the store and would scan more than %d resources", ErrTooMany, c.maxScan)
	}
	if c.params.Filter == nil || resource.EvalFilter(c.params.Filter, r) {
		c.matched = append(c.matched, r)
	}
	return nil
}

// Result sorts and paginates the matched resources.
func (c *ClientFilter) Result() *ListResult {
	matched := c.matched
	if c.params.SortBy != "" {
		order := resource.SortAscending
		if c.params.SortOrder == SortDescending {
			order = resource.SortDescending
		}
		resource.SortResources(matched, c.params.SortBy, order)
	}

	total := len(matched)
	startIndex := c.params.StartIndex
	if startIndex < 1 {
		startIndex = 1
	}
	start := startIndex - 1
	if start > total {
		start = total
	}
	pageSize := c.params.Count
	if pageSize < 0 {
		pageSize = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	page := matched[start:end]

	return &ListResult{
		Resources:    page,
		TotalResults: total,
		StartIndex:   startIndex,
		ItemsPerPage: len(page),
	}
}
