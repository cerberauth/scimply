package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/cerberauth/scimply/audit"
	"github.com/cerberauth/scimply/protocol"
	"github.com/cerberauth/scimply/resource"
	"github.com/cerberauth/scimply/store"
)

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request, resourceType string) {
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		protocol.NewSCIMError(http.StatusBadRequest, protocol.ErrTypeInvalidSyntax, "invalid JSON body: "+err.Error()).Write(w)
		return
	}

	res := resource.FromMap(body)

	created, err := s.cfg.store.Create(r.Context(), resourceType, res)
	if err != nil {
		scimErr := protocol.ErrorFromStoreError(err)
		s.logAudit(r, audit.OperationCreate, resourceType, "", httpStatus(scimErr))
		scimErr.Write(w)
		return
	}

	location := s.resourceLocation(r, resourceType, created.ID)
	if location != "" {
		created.Meta.Location = location
	}

	etag := etagFromVersion(created.Meta.Version)
	s.logAudit(r, audit.OperationCreate, resourceType, created.ID, http.StatusCreated)
	protocol.WriteCreated(w, r, created.ToMap(), location, etag)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request, resourceType, id string) {
	res, err := s.cfg.store.Get(r.Context(), resourceType, id)
	if err != nil {
		scimErr := protocol.ErrorFromStoreError(err)
		s.logAudit(r, audit.OperationRead, resourceType, id, httpStatus(scimErr))
		scimErr.Write(w)
		return
	}

	location := s.resourceLocation(r, resourceType, res.ID)
	if location != "" {
		res.Meta.Location = location
	}

	etag := etagFromVersion(res.Meta.Version)
	s.logAudit(r, audit.OperationRead, resourceType, id, http.StatusOK)
	protocol.WriteResource(w, r, res.ToMap(), location, etag)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request, resourceType string) {
	params := protocol.ParseListParams(r)

	if r.Method == http.MethodPost && r.Body != nil {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			bodyVals := bodyToURLValues(body)
			bodyParams := protocol.ParseListParamsFromValues(bodyVals)

			if bodyParams.Filter != "" {
				params.Filter = bodyParams.Filter
			}
			if bodyParams.SortBy != "" {
				params.SortBy = bodyParams.SortBy
			}
			if bodyParams.SortOrder != "" {
				params.SortOrder = bodyParams.SortOrder
			}
			if bodyParams.StartIndex > 1 {
				params.StartIndex = bodyParams.StartIndex
			}
			if bodyParams.Count > 0 {
				params.Count = bodyParams.Count
			}
		}
	}

	if params.Count <= 0 {
		// RFC 7644 §3.4.2.4: count=0 means "return no resource results, only totalResults".
		// Only apply the default page size when count was not explicitly provided as 0.
		if r.URL.Query().Get("count") != "0" {
			params.Count = s.cfg.defaultPageSize
		}
	}
	if s.cfg.maxPageSize > 0 && params.Count > s.cfg.maxPageSize {
		params.Count = s.cfg.maxPageSize
	}

	storeParams := store.ListParams{
		SortBy:             params.SortBy,
		StartIndex:         params.StartIndex,
		Count:              params.Count,
		Attributes:         params.Attributes,
		ExcludedAttributes: params.ExcludedAttributes,
	}
	if params.SortOrder == "descending" {
		storeParams.SortOrder = store.SortDescending
	}

	if params.Filter != "" {
		filterExpr, err := resource.ParseFilter(params.Filter)
		if err != nil {
			scimErr := protocol.NewSCIMError(http.StatusBadRequest, protocol.ErrTypeInvalidFilter, "invalid filter: "+err.Error())
			s.logAudit(r, audit.OperationList, resourceType, "", http.StatusBadRequest)
			scimErr.Write(w)
			return
		}
		storeParams.Filter = filterExpr
	}

	result, err := s.cfg.store.List(r.Context(), resourceType, storeParams)
	if err != nil {
		scimErr := protocol.ErrorFromStoreError(err)
		s.logAudit(r, audit.OperationList, resourceType, "", httpStatus(scimErr))
		scimErr.Write(w)
		return
	}

	resources := make([]interface{}, 0, len(result.Resources))
	for _, res := range result.Resources {
		location := s.resourceLocation(r, resourceType, res.ID)
		if location != "" {
			res.Meta.Location = location
		}
		resources = append(resources, res.ToMap())
	}

	listResp := protocol.NewListResponse(result.TotalResults, result.StartIndex, result.ItemsPerPage, resources)
	s.logAudit(r, audit.OperationList, resourceType, "", http.StatusOK)
	protocol.WriteJSON(w, http.StatusOK, listResp)
}

func (s *Server) handleReplace(w http.ResponseWriter, r *http.Request, resourceType, id string) {
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		protocol.NewSCIMError(http.StatusBadRequest, protocol.ErrTypeInvalidSyntax, "invalid JSON body: "+err.Error()).Write(w)
		return
	}

	res := resource.FromMap(body)

	replaced, err := s.cfg.store.Replace(r.Context(), resourceType, id, res)
	if err != nil {
		scimErr := protocol.ErrorFromStoreError(err)
		s.logAudit(r, audit.OperationReplace, resourceType, id, httpStatus(scimErr))
		scimErr.Write(w)
		return
	}

	location := s.resourceLocation(r, resourceType, replaced.ID)
	if location != "" {
		replaced.Meta.Location = location
	}

	etag := etagFromVersion(replaced.Meta.Version)
	s.logAudit(r, audit.OperationReplace, resourceType, id, http.StatusOK)
	protocol.WriteResource(w, r, replaced.ToMap(), location, etag)
}

func (s *Server) handlePatch(w http.ResponseWriter, r *http.Request, resourceType, id string) {
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		protocol.NewSCIMError(http.StatusBadRequest, protocol.ErrTypeInvalidSyntax, "invalid JSON body: "+err.Error()).Write(w)
		return
	}

	patchReq, err := resource.ParsePatchRequest(body)
	if err != nil {
		scimErr := protocol.ErrorFromStoreError(err)
		s.logAudit(r, audit.OperationPatch, resourceType, id, httpStatus(scimErr))
		scimErr.Write(w)
		return
	}

	patched, err := s.cfg.store.Patch(r.Context(), resourceType, id, patchReq.Operations)
	if err != nil {
		scimErr := protocol.ErrorFromStoreError(err)
		s.logAudit(r, audit.OperationPatch, resourceType, id, httpStatus(scimErr))
		scimErr.Write(w)
		return
	}

	location := s.resourceLocation(r, resourceType, patched.ID)
	if location != "" {
		patched.Meta.Location = location
	}

	etag := etagFromVersion(patched.Meta.Version)
	s.logAudit(r, audit.OperationPatch, resourceType, id, http.StatusOK)
	protocol.WriteResource(w, r, patched.ToMap(), location, etag)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request, resourceType, id string) {
	err := s.cfg.store.Delete(r.Context(), resourceType, id)
	if err != nil {
		scimErr := protocol.ErrorFromStoreError(err)
		s.logAudit(r, audit.OperationDelete, resourceType, id, httpStatus(scimErr))
		scimErr.Write(w)
		return
	}

	s.logAudit(r, audit.OperationDelete, resourceType, id, http.StatusNoContent)
	protocol.WriteNoContent(w)
}

func (s *Server) logAudit(r *http.Request, op audit.Operation, resourceType, resourceID string, statusCode int) {
	event := audit.Event{
		Timestamp:    time.Now().UTC(),
		Operation:    op,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		StatusCode:   statusCode,
		RequestID:    r.Header.Get("X-Request-ID"),
	}
	_ = s.cfg.auditLogger.Log(r.Context(), event)
}

func httpStatus(e *protocol.SCIMError) int {
	if e == nil {
		return http.StatusOK
	}
	code := http.StatusInternalServerError
	_, _ = fmt.Sscanf(e.Status, "%d", &code)
	return code
}

func bodyToURLValues(body map[string]interface{}) url.Values {
	vals := make(url.Values)
	for k, v := range body {
		switch sv := v.(type) {
		case string:
			vals.Set(k, sv)
		case float64:
			vals.Set(k, fmt.Sprintf("%g", sv))
		}
	}
	return vals
}
