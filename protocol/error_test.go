package protocol

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cerberauth/scimply/store"
)

func TestNewSCIMError(t *testing.T) {
	e := NewSCIMError(http.StatusBadRequest, ErrTypeInvalidFilter, "bad filter")
	if e.Status != "400" {
		t.Errorf("Status = %q, want %q", e.Status, "400")
	}
	if e.SCIMType != ErrTypeInvalidFilter {
		t.Errorf("SCIMType = %q, want %q", e.SCIMType, ErrTypeInvalidFilter)
	}
	if e.Detail != "bad filter" {
		t.Errorf("Detail = %q, want %q", e.Detail, "bad filter")
	}
	if len(e.Schemas) != 1 || e.Schemas[0] != scimErrorSchema {
		t.Errorf("Schemas = %v, want [%q]", e.Schemas, scimErrorSchema)
	}
}

func TestSCIMError_Error(t *testing.T) {
	e := NewSCIMError(http.StatusNotFound, "", "not found")
	if e.Error() == "" {
		t.Error("Error() should return non-empty string")
	}

	e2 := NewSCIMError(http.StatusInternalServerError, "", "")
	if e2.Error() == "" {
		t.Error("Error() without detail should return non-empty string")
	}
}

func TestErrorFromStoreError(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus string
		wantType   SCIMType
	}{
		{store.ErrNotFound, "404", ""},
		{store.ErrConflict, "409", ErrTypeUniqueness},
		{store.ErrMutability, "400", ErrTypeMutability},
		{store.ErrBadFilter, "400", ErrTypeInvalidFilter},
		{store.ErrBadPath, "400", ErrTypeInvalidPath},
		{store.ErrBadPatch, "400", ErrTypeInvalidSyntax},
		{store.ErrNoTarget, "400", ErrTypeNoTarget},
		{store.ErrTooMany, "413", ErrTypeTooMany},
		{store.ErrInvalidValue, "400", ErrTypeInvalidValue},
		{store.ErrInternal, "500", ""},
	}

	for _, tc := range tests {
		got := ErrorFromStoreError(tc.err)
		if got == nil {
			t.Errorf("ErrorFromStoreError(%v) = nil, want non-nil", tc.err)
			continue
		}
		if got.Status != tc.wantStatus {
			t.Errorf("ErrorFromStoreError(%v).Status = %q, want %q", tc.err, got.Status, tc.wantStatus)
		}
		if got.SCIMType != tc.wantType {
			t.Errorf("ErrorFromStoreError(%v).SCIMType = %q, want %q", tc.err, got.SCIMType, tc.wantType)
		}
	}

	wrapped := fmt.Errorf("wrapped: %w", store.ErrNotFound)
	got := ErrorFromStoreError(wrapped)
	if got.Status != "404" {
		t.Errorf("wrapped ErrNotFound status = %q, want 404", got.Status)
	}

	if ErrorFromStoreError(nil) != nil {
		t.Error("ErrorFromStoreError(nil) should return nil")
	}

	unknown := fmt.Errorf("something unexpected")
	got = ErrorFromStoreError(unknown)
	if got.Status != "500" {
		t.Errorf("unknown error status = %q, want 500", got.Status)
	}
}
