package scimconnector_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	scimconnector "github.com/cerberauth/scimply/connector/scim"
	"github.com/cerberauth/scimply/resource"
	"github.com/cerberauth/scimply/store"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func newTestClient(t *testing.T, srv *httptest.Server) *scimconnector.Client {
	t.Helper()
	c, err := scimconnector.New(
		scimconnector.WithBaseURL(srv.URL),
		scimconnector.WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func sampleUserMap(id string) map[string]interface{} {
	m := map[string]interface{}{
		"schemas":  []interface{}{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"id":       id,
		"userName": "jdoe",
	}
	return m
}

func sampleUserResource(id string) *resource.Resource {
	return resource.FromMap(sampleUserMap(id))
}

func TestCreate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/Users" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		writeJSON(w, http.StatusCreated, sampleUserMap("abc123"))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	res, err := c.Create(context.Background(), "User", sampleUserResource(""))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.ID != "abc123" {
		t.Errorf("got ID %q, want %q", res.ID, "abc123")
	}
}

func TestCreate_Conflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"schemas":  []interface{}{"urn:ietf:params:scim:api:messages:2.0:Error"},
			"status":   "409",
			"scimType": "uniqueness",
			"detail":   "userName already exists",
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Create(context.Background(), "User", sampleUserResource(""))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isErr(err, store.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestGet_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/Users/u1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		writeJSON(w, http.StatusOK, sampleUserMap("u1"))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	res, err := c.Get(context.Background(), "User", "u1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if res.ID != "u1" {
		t.Errorf("got ID %q, want %q", res.ID, "u1")
	}
}

func TestGet_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"schemas": []interface{}{"urn:ietf:params:scim:api:messages:2.0:Error"},
			"status":  "404",
			"detail":  "Resource not found",
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Get(context.Background(), "User", "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isErr(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestList_WithFilter(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"totalResults": 1,
			"startIndex":   1,
			"itemsPerPage": 1,
			"Resources":    []interface{}{sampleUserMap("u1")},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)

	filter, err := resource.ParseFilter(`userName eq "jdoe"`)
	if err != nil {
		t.Fatalf("ParseFilter: %v", err)
	}

	result, err := c.List(context.Background(), "User", store.ListParams{
		Filter:     filter,
		StartIndex: 1,
		Count:      10,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.TotalResults != 1 {
		t.Errorf("TotalResults = %d, want 1", result.TotalResults)
	}
	if len(result.Resources) != 1 {
		t.Errorf("len(Resources) = %d, want 1", len(result.Resources))
	}
	if gotQuery == "" {
		t.Error("expected query string to be set")
	}
}

func TestList_Pagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("startIndex") != "2" {
			t.Errorf("startIndex = %q, want %q", q.Get("startIndex"), "2")
		}
		if q.Get("count") != "5" {
			t.Errorf("count = %q, want %q", q.Get("count"), "5")
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"totalResults": 10,
			"startIndex":   2,
			"itemsPerPage": 5,
			"Resources":    []interface{}{},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	result, err := c.List(context.Background(), "User", store.ListParams{StartIndex: 2, Count: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.TotalResults != 10 {
		t.Errorf("TotalResults = %d, want 10", result.TotalResults)
	}
}

func TestReplace_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/Users/u1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		m := sampleUserMap("u1")
		m["userName"] = "jdoe_updated"
		writeJSON(w, http.StatusOK, m)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	res, err := c.Replace(context.Background(), "User", "u1", sampleUserResource("u1"))
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if res.ID != "u1" {
		t.Errorf("got ID %q, want %q", res.ID, "u1")
	}
}

func TestPatch_Success200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/Users/u1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		m := sampleUserMap("u1")
		m["displayName"] = "John Doe"
		writeJSON(w, http.StatusOK, m)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	ops := []resource.PatchOp{
		{Op: resource.PatchOpAdd, Value: map[string]interface{}{"displayName": "John Doe"}},
	}
	res, err := c.Patch(context.Background(), "User", "u1", ops)
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if res.ID != "u1" {
		t.Errorf("got ID %q, want %q", res.ID, "u1")
	}
}

func TestPatch_Success204(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {

			w.WriteHeader(http.StatusNoContent)
			return
		}

		writeJSON(w, http.StatusOK, sampleUserMap("u1"))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	ops := []resource.PatchOp{
		{Op: resource.PatchOpReplace, Value: map[string]interface{}{"displayName": "Jane"}},
	}
	res, err := c.Patch(context.Background(), "User", "u1", ops)
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if res.ID != "u1" {
		t.Errorf("got ID %q, want %q", res.ID, "u1")
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls (PATCH + GET), got %d", callCount)
	}
}

func TestDelete_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/Users/u1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.Delete(context.Background(), "User", "u1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"schemas": []interface{}{"urn:ietf:params:scim:api:messages:2.0:Error"},
			"status":  "404",
			"detail":  "not found",
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	err := c.Delete(context.Background(), "User", "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isErr(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestErrorMapping_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"status": "404"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Get(context.Background(), "User", "x")
	if !isErr(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestErrorMapping_409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusConflict, map[string]interface{}{"status": "409", "scimType": "uniqueness"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Create(context.Background(), "User", sampleUserResource(""))
	if !isErr(err, store.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestErrorMapping_400_InvalidFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"status":   "400",
			"scimType": "invalidFilter",
			"detail":   "bad filter",
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.List(context.Background(), "User", store.ListParams{})
	if !isErr(err, store.ErrBadFilter) {
		t.Errorf("expected ErrBadFilter, got %v", err)
	}
}

func isErr(err, target error) bool {
	if err == nil {
		return false
	}

	type unwrapper interface{ Unwrap() error }
	for {
		if err == target {
			return true
		}
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return false
}
