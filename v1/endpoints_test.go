package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteV1Error_Format(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteV1Error(rr, http.StatusNotFound, "Resource not found")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rr.Code)
	}

	var resp V1ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(resp.Errors))
	}
	if resp.Errors[0].Description != "Resource not found" {
		t.Errorf("description: got %q, want %q", resp.Errors[0].Description, "Resource not found")
	}
	if len(resp.Schemas) == 0 {
		t.Error("expected schemas to be set")
	}
	if resp.Schemas[0] != CoreSchemaURI {
		t.Errorf("schemas[0]: got %q, want %q", resp.Schemas[0], CoreSchemaURI)
	}
}

func TestWriteV1Error_ContentType(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteV1Error(rr, http.StatusBadRequest, "Bad input")

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}
