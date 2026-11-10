package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cerberauth/scimply/resource"
)

func newCFUser(name string) *resource.Resource {
	return &resource.Resource{
		ID:         name,
		Attributes: map[string]interface{}{"userName": name},
	}
}

func TestClientFilter_FilterSortPaginate(t *testing.T) {
	f, err := resource.ParseFilter(`userName sw "a"`)
	if err != nil {
		t.Fatal(err)
	}
	cf := NewClientFilter(ListParams{
		Filter:     f,
		SortBy:     "userName",
		SortOrder:  SortDescending,
		StartIndex: 2,
		Count:      2,
	}, 0)
	for _, n := range []string{"a1", "b1", "a2", "a3", "b2", "a4"} {
		if err := cf.Add(newCFUser(n)); err != nil {
			t.Fatal(err)
		}
	}
	res := cf.Result()
	if res.TotalResults != 4 {
		t.Errorf("TotalResults: got %d, want 4", res.TotalResults)
	}
	if res.StartIndex != 2 || res.ItemsPerPage != 2 {
		t.Errorf("StartIndex/ItemsPerPage: got %d/%d, want 2/2", res.StartIndex, res.ItemsPerPage)
	}
	if got := []string{res.Resources[0].ID, res.Resources[1].ID}; got[0] != "a3" || got[1] != "a2" {
		t.Errorf("page: got %v, want [a3 a2]", got)
	}
}

func TestClientFilter_NoTruncation(t *testing.T) {
	cf := NewClientFilter(ListParams{Count: 10}, 0)
	for i := 0; i < 2500; i++ {
		if err := cf.Add(newCFUser(fmt.Sprintf("u%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	if res := cf.Result(); res.TotalResults != 2500 || res.ItemsPerPage != 10 {
		t.Errorf("got total=%d items=%d, want 2500/10", res.TotalResults, res.ItemsPerPage)
	}
}

func TestClientFilter_MaxScan(t *testing.T) {
	cf := NewClientFilter(ListParams{}, 3)
	for i := 0; i < 3; i++ {
		if err := cf.Add(newCFUser(fmt.Sprintf("u%d", i))); err != nil {
			t.Fatalf("Add %d: %v", i, err)
		}
	}
	if err := cf.Add(newCFUser("u3")); !errors.Is(err, ErrTooMany) {
		t.Errorf("expected ErrTooMany, got %v", err)
	}
}

func TestClientFilter_CountZero(t *testing.T) {
	cf := NewClientFilter(ListParams{Count: 0}, 0)
	_ = cf.Add(newCFUser("a"))
	if res := cf.Result(); res.TotalResults != 1 || len(res.Resources) != 0 {
		t.Errorf("count=0: got total=%d items=%d, want 1/0", res.TotalResults, len(res.Resources))
	}
}
