package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/cerberauth/scimply/store"
)

type SCIMType string

const (
	ErrTypeInvalidFilter SCIMType = "invalidFilter"
	ErrTypeTooMany       SCIMType = "tooMany"
	ErrTypeUniqueness    SCIMType = "uniqueness"
	ErrTypeMutability    SCIMType = "mutability"
	ErrTypeInvalidSyntax SCIMType = "invalidSyntax"
	ErrTypeInvalidPath   SCIMType = "invalidPath"
	ErrTypeNoTarget      SCIMType = "noTarget"
	ErrTypeInvalidValue  SCIMType = "invalidValue"
	ErrTypeInvalidVers   SCIMType = "invalidVers"
	ErrTypeSensitive     SCIMType = "sensitive"
)

const scimErrorSchema = "urn:ietf:params:scim:api:messages:2.0:Error"

type SCIMError struct {
	Schemas  []string `json:"schemas"`
	Status   string   `json:"status"`
	SCIMType SCIMType `json:"scimType,omitempty"`
	Detail   string   `json:"detail,omitempty"`
}

func NewSCIMError(status int, scimType SCIMType, detail string) *SCIMError {
	return &SCIMError{
		Schemas:  []string{scimErrorSchema},
		Status:   fmt.Sprintf("%d", status),
		SCIMType: scimType,
		Detail:   detail,
	}
}

func (e *SCIMError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("SCIM %s: %s", e.Status, e.Detail)
	}
	return fmt.Sprintf("SCIM %s", e.Status)
}

func (e *SCIMError) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", ContentTypeSCIM)
	statusCode := http.StatusInternalServerError
	_, _ = fmt.Sscanf(e.Status, "%d", &statusCode)
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(e)
}

func ErrorFromStoreError(err error) *SCIMError {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, store.ErrNotFound):
		return NewSCIMError(http.StatusNotFound, "", err.Error())

	case errors.Is(err, store.ErrConflict):
		return NewSCIMError(http.StatusConflict, ErrTypeUniqueness, err.Error())

	case errors.Is(err, store.ErrMutability):
		return NewSCIMError(http.StatusBadRequest, ErrTypeMutability, err.Error())

	case errors.Is(err, store.ErrBadFilter):
		return NewSCIMError(http.StatusBadRequest, ErrTypeInvalidFilter, err.Error())

	case errors.Is(err, store.ErrBadPath):
		return NewSCIMError(http.StatusBadRequest, ErrTypeInvalidPath, err.Error())

	case errors.Is(err, store.ErrBadPatch):
		return NewSCIMError(http.StatusBadRequest, ErrTypeInvalidSyntax, err.Error())

	case errors.Is(err, store.ErrNoTarget):
		return NewSCIMError(http.StatusBadRequest, ErrTypeNoTarget, err.Error())

	case errors.Is(err, store.ErrTooMany):
		return NewSCIMError(http.StatusRequestEntityTooLarge, ErrTypeTooMany, err.Error())

	case errors.Is(err, store.ErrInvalidValue):
		return NewSCIMError(http.StatusBadRequest, ErrTypeInvalidValue, err.Error())

	case errors.Is(err, store.ErrInternal):
		return NewSCIMError(http.StatusInternalServerError, "", err.Error())

	default:
		return NewSCIMError(http.StatusInternalServerError, "", err.Error())
	}
}
