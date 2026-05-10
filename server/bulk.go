package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/cerberauth/scimply/protocol"
	"github.com/cerberauth/scimply/resource"
)

const (
	schemasKey         = "schemas"
	bulkRequestSchema  = "urn:ietf:params:scim:api:messages:2.0:BulkRequest"
	bulkResponseSchema = "urn:ietf:params:scim:api:messages:2.0:BulkResponse"
)

type bulkResponseOperation struct {
	Method   string      `json:"method"`
	BulkID   string      `json:"bulkId,omitempty"`
	Location string      `json:"location,omitempty"`
	Version  string      `json:"version,omitempty"`
	Status   string      `json:"status"`
	Response interface{} `json:"response,omitempty"`
}

type bulkResponse struct {
	Schemas    []string                `json:"schemas"`
	Operations []bulkResponseOperation `json:"Operations"`
}

func (s *Server) handleBulk(w http.ResponseWriter, r *http.Request) {

	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		protocol.NewSCIMError(http.StatusBadRequest, protocol.ErrTypeInvalidSyntax, "invalid JSON body: "+err.Error()).Write(w)
		return
	}

	bulkReq, err := resource.ParseBulkRequest(body)
	if err != nil {
		protocol.NewSCIMError(http.StatusBadRequest, protocol.ErrTypeInvalidSyntax, "invalid bulk request: "+err.Error()).Write(w)
		return
	}

	if s.cfg.maxBulkOps > 0 && len(bulkReq.Operations) > s.cfg.maxBulkOps {
		protocol.NewSCIMError(http.StatusRequestEntityTooLarge, protocol.ErrTypeTooMany,
			fmt.Sprintf("bulk request exceeds maxOperations limit of %d", s.cfg.maxBulkOps)).Write(w)
		return
	}

	ops := make([]bulkResponseOperation, 0, len(bulkReq.Operations))
	errorCount := 0

	for _, op := range bulkReq.Operations {
		result := s.executeBulkOp(r, op)
		ops = append(ops, result)

		// Parse the string status code back to an integer to check for errors.
		statusCode := 0
		_, _ = fmt.Sscanf(result.Status, "%d", &statusCode)
		if statusCode >= 300 {
			errorCount++
		}

		// FailOnErrors is the RFC 7644 §3.7 threshold: stop processing once
		// this many operations have failed.
		if bulkReq.FailOnErrors > 0 && errorCount >= bulkReq.FailOnErrors {
			break
		}
	}

	resp := &bulkResponse{
		Schemas:    []string{bulkResponseSchema},
		Operations: ops,
	}

	protocol.WriteJSON(w, http.StatusOK, resp)
}

// executeBulkOp executes a single bulk operation by constructing a real
// http.Request and dispatching it through the server's own router via an
// httptest.ResponseRecorder. This approach reuses all existing handler logic
// (auth, validation, routing) without code duplication.
//
// Key details:
//   - Headers and TLS state are cloned from the original request so that
//     authentication middleware sees the same credentials.
//   - The operation body (op.Data) is re-marshalled to JSON to form the request
//     body, and Content-Type is forced to the SCIM media type.
//   - The response recorder captures the status code, Location/ETag headers,
//     and body, which are then included in the bulk response entry.
func (s *Server) executeBulkOp(origReq *http.Request, op resource.BulkOperation) bulkResponseOperation {
	result := bulkResponseOperation{
		Method: string(op.Method),
		BulkID: op.BulkID,
	}

	method := strings.ToUpper(string(op.Method))
	path := op.Path
	if path == "" {
		result.Status = "400"
		result.Response = map[string]interface{}{
			schemasKey: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			"status":   "400",
			"detail":   "missing path in bulk operation",
			"scimType": string(protocol.ErrTypeInvalidSyntax),
		}
		return result
	}

	fullPath := s.cfg.basePath + path

	var bodyReader io.Reader
	if op.Data != nil {
		data, err := json.Marshal(op.Data)
		if err != nil {
			result.Status = "400"
			result.Response = errorMap(http.StatusBadRequest, protocol.ErrTypeInvalidSyntax, "cannot marshal bulk op data: "+err.Error())
			return result
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(origReq.Context(), method, fullPath, bodyReader)
	if err != nil {
		result.Status = "400"
		result.Response = errorMap(http.StatusBadRequest, "", "cannot build bulk op request: "+err.Error())
		return result
	}

	// Propagate auth and other headers from the outer request.
	req.Header = origReq.Header.Clone()
	req.Host = origReq.Host
	req.TLS = origReq.TLS
	if op.Data != nil {
		req.Header.Set("Content-Type", protocol.ContentTypeSCIM)
	}
	req.URL.Path = fullPath

	// Dispatch through the normal routing tree and capture the response.
	rr := httptest.NewRecorder()
	s.route(rr, req)

	statusCode := rr.Code
	result.Status = fmt.Sprintf("%d", statusCode)

	if loc := rr.Header().Get("Location"); loc != "" {
		result.Location = loc
	}
	if etag := rr.Header().Get("ETag"); etag != "" {
		result.Version = etag
	}

	if rr.Body.Len() > 0 {
		var respBody interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &respBody); err == nil {
			result.Response = respBody
		}
	}

	return result
}

func errorMap(status int, scimType protocol.SCIMType, detail string) map[string]interface{} {
	m := map[string]interface{}{
		schemasKey: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
		"status":   fmt.Sprintf("%d", status),
		"detail":   detail,
	}
	if scimType != "" {
		m["scimType"] = string(scimType)
	}
	return m
}
