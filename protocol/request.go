package protocol

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type RequestType int

const (
	RequestCreate RequestType = iota
	RequestRead
	RequestReplace
	RequestPatch
	RequestDelete
	RequestList
	RequestSearch
	RequestBulk
	RequestDiscovery
)

type SCIMRequest struct {
	Type         RequestType
	Version      Version
	ResourceType string
	ResourceID   string
	Params       ListParams
	Body         map[string]interface{}
}

func ParseRequest(r *http.Request, basePath string) (*SCIMRequest, error) {
	req := &SCIMRequest{
		Version: DetectVersion(r.URL.Path, r.Header.Get("Content-Type")),
	}

	path := r.URL.Path
	if basePath != "" {
		trimmed := strings.TrimPrefix(path, basePath)
		if trimmed != path {
			path = trimmed
		}
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	segments := splitPath(path)

	if len(segments) == 0 || (len(segments) == 1 && segments[0] == "") {
		req.Type = RequestDiscovery
		return req, nil
	}

	first := segments[0]

	switch strings.ToLower(first) {
	case "serviceproviderconfig", "serviceproviderconfigs",
		"resourcetypes", "schemas":
		req.Type = RequestDiscovery
		return req, nil
	}

	if strings.ToLower(first) == "bulk" {
		req.Type = RequestBulk
		if err := decodeBody(r, req); err != nil {
			return nil, err
		}
		return req, nil
	}

	if first == ".search" {
		req.Type = RequestSearch
		req.Params = ParseListParams(r)
		if err := decodeBody(r, req); err != nil {
			return nil, err
		}
		return req, nil
	}

	req.ResourceType = resourceTypeName(first)

	if len(segments) == 1 {

		switch r.Method {
		case http.MethodGet:
			req.Type = RequestList
			req.Params = ParseListParams(r)
		case http.MethodPost:
			req.Type = RequestCreate
			if err := decodeBody(r, req); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("method %s not allowed on collection", r.Method)
		}
		return req, nil
	}

	second := segments[1]

	if second == ".search" {
		req.Type = RequestSearch
		req.Params = ParseListParams(r)
		if err := decodeBody(r, req); err != nil {
			return nil, err
		}
		return req, nil
	}

	req.ResourceID = second
	switch r.Method {
	case http.MethodGet:
		req.Type = RequestRead
		req.Params = ParseListParams(r)
	case http.MethodPut:
		req.Type = RequestReplace
		if err := decodeBody(r, req); err != nil {
			return nil, err
		}
	case http.MethodPatch:
		req.Type = RequestPatch
		if err := decodeBody(r, req); err != nil {
			return nil, err
		}
	case http.MethodDelete:
		req.Type = RequestDelete
	default:
		return nil, fmt.Errorf("method %s not allowed on resource", r.Method)
	}

	return req, nil
}

func splitPath(p string) []string {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil
	}
	return strings.SplitN(p, "/", 2)
}

func resourceTypeName(segment string) string {
	switch strings.ToLower(segment) {
	case "users":
		return "User"
	case "groups":
		return "Group"
	}

	if strings.HasSuffix(segment, "s") {
		return segment[:len(segment)-1]
	}
	return segment
}

func decodeBody(r *http.Request, req *SCIMRequest) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	req.Body = body
	return nil
}
