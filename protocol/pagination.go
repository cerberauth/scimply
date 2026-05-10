package protocol

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const scimListResponseSchema = "urn:ietf:params:scim:api:messages:2.0:ListResponse"

type ListParams struct {
	Filter             string
	SortBy             string
	SortOrder          string
	StartIndex         int
	Count              int
	Attributes         []string
	ExcludedAttributes []string
}

func ParseListParams(r *http.Request) ListParams {
	return ParseListParamsFromValues(r.URL.Query())
}

func ParseListParamsFromValues(v url.Values) ListParams {
	params := ListParams{
		StartIndex: 1,
	}

	if filter := v.Get("filter"); filter != "" {
		params.Filter = filter
	}

	if sortBy := v.Get("sortBy"); sortBy != "" {
		params.SortBy = sortBy
	}

	if sortOrder := strings.ToLower(v.Get("sortOrder")); sortOrder == "descending" {
		params.SortOrder = "descending"
	} else if sortOrder == "ascending" {
		params.SortOrder = "ascending"
	}

	if si := v.Get("startIndex"); si != "" {
		if n, err := strconv.Atoi(si); err == nil {
			if n < 1 {
				n = 1
			}
			params.StartIndex = n
		}
	}

	if count := v.Get("count"); count != "" {
		if n, err := strconv.Atoi(count); err == nil && n >= 0 {
			params.Count = n
		}
	}

	if attrs := v.Get("attributes"); attrs != "" {
		for _, a := range strings.Split(attrs, ",") {
			a = strings.TrimSpace(a)
			if a != "" {
				params.Attributes = append(params.Attributes, a)
			}
		}
	}

	if excl := v.Get("excludedAttributes"); excl != "" {
		for _, a := range strings.Split(excl, ",") {
			a = strings.TrimSpace(a)
			if a != "" {
				params.ExcludedAttributes = append(params.ExcludedAttributes, a)
			}
		}
	}

	return params
}

type ListResponse struct {
	Schemas      []string      `json:"schemas"`
	TotalResults int           `json:"totalResults"`
	StartIndex   int           `json:"startIndex"`
	ItemsPerPage int           `json:"itemsPerPage"`
	Resources    []interface{} `json:"Resources"`
}

func NewListResponse(totalResults, startIndex, itemsPerPage int, resources []interface{}) *ListResponse {
	if resources == nil {
		resources = []interface{}{}
	}
	return &ListResponse{
		Schemas:      []string{scimListResponseSchema},
		TotalResults: totalResults,
		StartIndex:   startIndex,
		ItemsPerPage: itemsPerPage,
		Resources:    resources,
	}
}
