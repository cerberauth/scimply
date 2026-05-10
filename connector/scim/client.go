package scimconnector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cerberauth/scimply/protocol"
	"github.com/cerberauth/scimply/resource"
	"github.com/cerberauth/scimply/store"
)

const (
	contentTypeSCIM = "application/scim+json"
	maxRetries      = 3
)

type Client struct {
	cfg ClientConfig
}

func New(opts ...Option) (*Client, error) {
	cfg := ClientConfig{
		Version: protocol.V2_0,
		Timeout: 30 * time.Second,
	}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.BaseURL == "" {
		return nil, errors.New("scimconnector: BaseURL is required")
	}

	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: cfg.Timeout}
	} else if cfg.Timeout > 0 {

		if cfg.HTTPClient.Timeout == 0 {
			cfg.HTTPClient.Timeout = cfg.Timeout
		}
	}
	return &Client{cfg: cfg}, nil
}

func (c *Client) resourceURL(resourceType string) string {
	return c.cfg.BaseURL + "/" + pluralize(resourceType)
}

func (c *Client) resourceItemURL(resourceType, id string) string {
	return c.resourceURL(resourceType) + "/" + url.PathEscape(id)
}

func pluralize(resourceType string) string {
	return resourceType + "s"
}

func (c *Client) newRequest(ctx context.Context, method, rawURL string, body interface{}) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("%w: marshal request body: %v", store.ErrInternal, err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", store.ErrInternal, err)
	}

	req.Header.Set("Content-Type", contentTypeSCIM)
	req.Header.Set("Accept", contentTypeSCIM)
	if c.cfg.AuthHeader != "" {
		req.Header.Set("Authorization", c.cfg.AuthHeader)
	}
	return req, nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {

	var bodyBytes []byte
	if req.Body != nil && req.Body != http.NoBody {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("%w: read request body: %v", store.ErrInternal, err)
		}
	}

	var resp *http.Response
	backoff := 500 * time.Millisecond
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if len(bodyBytes) > 0 {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		var err error
		resp, err = c.cfg.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%w: HTTP request failed: %v", store.ErrInternal, err)
		}

		if resp.StatusCode != http.StatusTooManyRequests {
			break
		}

		_ = resp.Body.Close()
		if attempt == maxRetries {
			return nil, fmt.Errorf("%w: rate limit exceeded after %d retries", store.ErrInternal, maxRetries)
		}

		wait := backoff
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				wait = time.Duration(secs) * time.Second
			}
		}
		select {
		case <-time.After(wait):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		backoff *= 2
	}
	return resp, nil
}

type scimErrorBody struct {
	SCIMType string `json:"scimType"`
	Detail   string `json:"detail"`
	Status   string `json:"status"`
}

func mapHTTPError(resp *http.Response) error {
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var scimErr scimErrorBody
	_ = json.Unmarshal(data, &scimErr)

	switch resp.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", store.ErrNotFound, scimErr.Detail)
	case http.StatusConflict:
		return fmt.Errorf("%w: %s", store.ErrConflict, scimErr.Detail)
	case http.StatusBadRequest:
		if strings.EqualFold(scimErr.SCIMType, "invalidFilter") {
			return fmt.Errorf("%w: %s", store.ErrBadFilter, scimErr.Detail)
		}
		if strings.EqualFold(scimErr.SCIMType, "invalidPath") {
			return fmt.Errorf("%w: %s", store.ErrBadPath, scimErr.Detail)
		}
		if strings.EqualFold(scimErr.SCIMType, "noTarget") {
			return fmt.Errorf("%w: %s", store.ErrNoTarget, scimErr.Detail)
		}
		return fmt.Errorf("%w: HTTP 400: %s", store.ErrInternal, scimErr.Detail)
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: HTTP %d: %s", store.ErrInternal, resp.StatusCode, scimErr.Detail)
	default:
		return fmt.Errorf("%w: HTTP %d: %s", store.ErrInternal, resp.StatusCode, scimErr.Detail)
	}
}

func decodeResource(resp *http.Response) (*resource.Resource, error) {
	defer resp.Body.Close()
	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", store.ErrInternal, err)
	}
	return resource.FromMap(m), nil
}

func (c *Client) Create(ctx context.Context, resourceType string, res *resource.Resource) (*resource.Resource, error) {
	req, err := c.newRequest(ctx, http.MethodPost, c.resourceURL(resourceType), res.ToMap())
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, mapHTTPError(resp)
	}

	return decodeResource(resp)
}

func (c *Client) Get(ctx context.Context, resourceType string, id string) (*resource.Resource, error) {
	req, err := c.newRequest(ctx, http.MethodGet, c.resourceItemURL(resourceType, id), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, mapHTTPError(resp)
	}

	return decodeResource(resp)
}

type listResponse struct {
	TotalResults int                      `json:"totalResults"`
	StartIndex   int                      `json:"startIndex"`
	ItemsPerPage int                      `json:"itemsPerPage"`
	Resources    []map[string]interface{} `json:"Resources"`
}

func (c *Client) List(ctx context.Context, resourceType string, params store.ListParams) (*store.ListResult, error) {
	u, err := url.Parse(c.resourceURL(resourceType))
	if err != nil {
		return nil, fmt.Errorf("%w: build list URL: %v", store.ErrInternal, err)
	}

	q := u.Query()
	if params.Filter != nil {
		q.Set("filter", filterToString(params.Filter))
	}
	if params.StartIndex > 0 {
		q.Set("startIndex", strconv.Itoa(params.StartIndex))
	}
	if params.Count > 0 {
		q.Set("count", strconv.Itoa(params.Count))
	}
	if params.SortBy != "" {
		q.Set("sortBy", params.SortBy)
		if params.SortOrder == store.SortDescending {
			q.Set("sortOrder", "descending")
		} else {
			q.Set("sortOrder", "ascending")
		}
	}
	u.RawQuery = q.Encode()

	req, err := c.newRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, mapHTTPError(resp)
	}
	defer resp.Body.Close()

	var lr listResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, fmt.Errorf("%w: decode list response: %v", store.ErrInternal, err)
	}

	resources := make([]*resource.Resource, 0, len(lr.Resources))
	for _, m := range lr.Resources {
		resources = append(resources, resource.FromMap(m))
	}

	startIndex := lr.StartIndex
	if startIndex == 0 {
		startIndex = 1
	}

	return &store.ListResult{
		Resources:    resources,
		TotalResults: lr.TotalResults,
		StartIndex:   startIndex,
		ItemsPerPage: lr.ItemsPerPage,
	}, nil
}

func (c *Client) Replace(ctx context.Context, resourceType string, id string, res *resource.Resource) (*resource.Resource, error) {
	req, err := c.newRequest(ctx, http.MethodPut, c.resourceItemURL(resourceType, id), res.ToMap())
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, mapHTTPError(resp)
	}

	return decodeResource(resp)
}

type patchBody struct {
	Schemas    []string      `json:"schemas"`
	Operations []patchOpWire `json:"Operations"`
}

type patchOpWire struct {
	Op    string      `json:"op"`
	Path  string      `json:"path,omitempty"`
	Value interface{} `json:"value,omitempty"`
}

func (c *Client) Patch(ctx context.Context, resourceType string, id string, ops []resource.PatchOp) (*resource.Resource, error) {
	wireOps := make([]patchOpWire, 0, len(ops))
	for _, op := range ops {
		wo := patchOpWire{
			Op:    string(op.Op),
			Value: op.Value,
		}
		if op.Path != nil {
			wo.Path = patchPathToString(op.Path)
		}
		wireOps = append(wireOps, wo)
	}

	body := patchBody{
		Schemas:    []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		Operations: wireOps,
	}

	req, err := c.newRequest(ctx, http.MethodPatch, c.resourceItemURL(resourceType, id), body)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return decodeResource(resp)
	case http.StatusNoContent:
		_ = resp.Body.Close()

		return c.Get(ctx, resourceType, id)
	default:
		return nil, mapHTTPError(resp)
	}
}

func (c *Client) Delete(ctx context.Context, resourceType string, id string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, c.resourceItemURL(resourceType, id), nil)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusNoContent {
		return mapHTTPError(resp)
	}
	resp.Body.Close()
	return nil
}

func filterToString(expr resource.FilterExpression) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *resource.AttrExpression:
		path := e.Path.String()
		if e.Operator == resource.OpPr {
			return path + " pr"
		}
		return path + " " + e.Operator.String() + " " + formatFilterValue(e.Value)
	case *resource.LogicalExpression:
		left := filterToString(e.Left)
		right := filterToString(e.Right)
		op := "and"
		if e.Op == resource.LogicalOr {
			op = "or"
		}
		return "(" + left + " " + op + " " + right + ")"
	case *resource.NotExpression:
		return "not (" + filterToString(e.Inner) + ")"
	case *resource.ValuePathExpression:
		return e.Path.String() + "[" + filterToString(e.Filter) + "]"
	default:
		return ""
	}
}

func formatFilterValue(v interface{}) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case string:

		data, _ := json.Marshal(val)
		return string(data)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func patchPathToString(pp *resource.PatchPath) string {
	if pp == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(pp.Attribute.String())
	if pp.ValueFilter != nil {
		b.WriteByte('[')
		b.WriteString(filterToString(pp.ValueFilter))
		b.WriteByte(']')
	}
	if pp.SubAttribute != "" {
		b.WriteByte('.')
		b.WriteString(pp.SubAttribute)
	}
	return b.String()
}
