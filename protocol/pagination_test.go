package protocol

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func listRequest(query string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/Users?"+query, nil)
}

func TestParseListParams_Defaults(t *testing.T) {
	r := listRequest("")
	p := ParseListParams(r)
	if p.StartIndex != 1 {
		t.Errorf("default StartIndex = %d, want 1", p.StartIndex)
	}
	if p.Count != 0 {
		t.Errorf("default Count = %d, want 0", p.Count)
	}
	if p.Filter != "" {
		t.Errorf("default Filter = %q, want empty", p.Filter)
	}
	if p.SortBy != "" {
		t.Errorf("default SortBy = %q, want empty", p.SortBy)
	}
	if p.SortOrder != "" {
		t.Errorf("default SortOrder = %q, want empty", p.SortOrder)
	}
}

func TestParseListParams_StartIndexCount(t *testing.T) {
	r := listRequest("startIndex=2&count=10")
	p := ParseListParams(r)
	if p.StartIndex != 2 {
		t.Errorf("StartIndex = %d, want 2", p.StartIndex)
	}
	if p.Count != 10 {
		t.Errorf("Count = %d, want 10", p.Count)
	}
}

func TestParseListParams_ZeroStartIndex(t *testing.T) {

	r := listRequest("startIndex=0")
	p := ParseListParams(r)
	if p.StartIndex != 1 {
		t.Errorf("startIndex=0 → StartIndex = %d, want 1", p.StartIndex)
	}

	r2 := listRequest("startIndex=-5")
	p2 := ParseListParams(r2)
	if p2.StartIndex != 1 {
		t.Errorf("startIndex=-5 → StartIndex = %d, want 1", p2.StartIndex)
	}
}

func TestParseListParams_Filter(t *testing.T) {
	r := listRequest(`filter=userName+eq+"bjensen"`)
	p := ParseListParams(r)
	if p.Filter == "" {
		t.Error("Filter should not be empty")
	}
}

func TestParseListParams_SortBy(t *testing.T) {
	r := listRequest("sortBy=userName&sortOrder=descending")
	p := ParseListParams(r)
	if p.SortBy != "userName" {
		t.Errorf("SortBy = %q, want %q", p.SortBy, "userName")
	}
	if p.SortOrder != "descending" {
		t.Errorf("SortOrder = %q, want %q", p.SortOrder, "descending")
	}
}

func TestParseListParams_SortOrderAscending(t *testing.T) {
	r := listRequest("sortBy=name&sortOrder=ascending")
	p := ParseListParams(r)
	if p.SortOrder != "ascending" {
		t.Errorf("SortOrder = %q, want ascending", p.SortOrder)
	}
}

func TestParseListParams_Attributes(t *testing.T) {
	r := listRequest("attributes=userName,displayName")
	p := ParseListParams(r)
	if len(p.Attributes) != 2 {
		t.Errorf("len(Attributes) = %d, want 2", len(p.Attributes))
	}
	if p.Attributes[0] != "userName" {
		t.Errorf("Attributes[0] = %q, want userName", p.Attributes[0])
	}
	if p.Attributes[1] != "displayName" {
		t.Errorf("Attributes[1] = %q, want displayName", p.Attributes[1])
	}
}

func TestNewListResponse(t *testing.T) {
	resources := []interface{}{"a", "b"}
	lr := NewListResponse(100, 1, 2, resources)
	if lr.TotalResults != 100 {
		t.Errorf("TotalResults = %d, want 100", lr.TotalResults)
	}
	if lr.StartIndex != 1 {
		t.Errorf("StartIndex = %d, want 1", lr.StartIndex)
	}
	if lr.ItemsPerPage != 2 {
		t.Errorf("ItemsPerPage = %d, want 2", lr.ItemsPerPage)
	}
	if len(lr.Schemas) != 1 || lr.Schemas[0] != scimListResponseSchema {
		t.Errorf("Schemas = %v", lr.Schemas)
	}
	if len(lr.Resources) != 2 {
		t.Errorf("len(Resources) = %d, want 2", len(lr.Resources))
	}
}

func TestNewListResponse_NilResources(t *testing.T) {
	lr := NewListResponse(0, 1, 0, nil)
	if lr.Resources == nil {
		t.Error("Resources should not be nil")
	}
}
