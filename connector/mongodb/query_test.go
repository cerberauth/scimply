package mongodb

import (
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/cerberauth/scimply/resource"
)

func parseFilter(t *testing.T, s string) resource.FilterExpression {
	t.Helper()
	f, err := resource.ParseFilter(s)
	if err != nil {
		t.Fatalf("ParseFilter(%q): %v", s, err)
	}
	return f
}

func TestTranslateFilter_Eq(t *testing.T) {
	expr := parseFilter(t, `userName eq "alice"`)
	got, err := TranslateFilter(expr)
	if err != nil {
		t.Fatalf("TranslateFilter: %v", err)
	}
	want := bson.D{{Key: "userName", Value: "alice"}}
	if !bsonDEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTranslateFilter_Ne(t *testing.T) {
	expr := parseFilter(t, `userName ne "alice"`)
	got, err := TranslateFilter(expr)
	if err != nil {
		t.Fatalf("TranslateFilter: %v", err)
	}
	want := bson.D{{Key: "userName", Value: bson.D{{Key: "$ne", Value: "alice"}}}}
	if !bsonDEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTranslateFilter_Co(t *testing.T) {
	expr := parseFilter(t, `displayName co "Smith"`)
	got, err := TranslateFilter(expr)
	if err != nil {
		t.Fatalf("TranslateFilter: %v", err)
	}
	want := bson.D{{Key: "displayName", Value: bson.D{{Key: "$regex", Value: ".*Smith.*"}}}}
	if !bsonDEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTranslateFilter_Sw(t *testing.T) {
	expr := parseFilter(t, `userName sw "ali"`)
	got, err := TranslateFilter(expr)
	if err != nil {
		t.Fatalf("TranslateFilter: %v", err)
	}
	want := bson.D{{Key: "userName", Value: bson.D{{Key: "$regex", Value: "^ali"}}}}
	if !bsonDEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTranslateFilter_Ew(t *testing.T) {
	expr := parseFilter(t, `userName ew "ice"`)
	got, err := TranslateFilter(expr)
	if err != nil {
		t.Fatalf("TranslateFilter: %v", err)
	}
	want := bson.D{{Key: "userName", Value: bson.D{{Key: "$regex", Value: "ice$"}}}}
	if !bsonDEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTranslateFilter_Gt(t *testing.T) {
	expr := parseFilter(t, `age gt 30`)
	got, err := TranslateFilter(expr)
	if err != nil {
		t.Fatalf("TranslateFilter: %v", err)
	}
	want := bson.D{{Key: "age", Value: bson.D{{Key: "$gt", Value: float64(30)}}}}
	if !bsonDEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTranslateFilter_Ge(t *testing.T) {
	expr := parseFilter(t, `age ge 18`)
	got, err := TranslateFilter(expr)
	if err != nil {
		t.Fatalf("TranslateFilter: %v", err)
	}
	want := bson.D{{Key: "age", Value: bson.D{{Key: "$gte", Value: float64(18)}}}}
	if !bsonDEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTranslateFilter_Lt(t *testing.T) {
	expr := parseFilter(t, `age lt 65`)
	got, err := TranslateFilter(expr)
	if err != nil {
		t.Fatalf("TranslateFilter: %v", err)
	}
	want := bson.D{{Key: "age", Value: bson.D{{Key: "$lt", Value: float64(65)}}}}
	if !bsonDEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTranslateFilter_Le(t *testing.T) {
	expr := parseFilter(t, `age le 100`)
	got, err := TranslateFilter(expr)
	if err != nil {
		t.Fatalf("TranslateFilter: %v", err)
	}
	want := bson.D{{Key: "age", Value: bson.D{{Key: "$lte", Value: float64(100)}}}}
	if !bsonDEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTranslateFilter_Pr(t *testing.T) {
	expr := parseFilter(t, `email pr`)
	got, err := TranslateFilter(expr)
	if err != nil {
		t.Fatalf("TranslateFilter: %v", err)
	}
	want := bson.D{{Key: "email", Value: bson.D{
		{Key: "$exists", Value: true},
		{Key: "$ne", Value: nil},
	}}}
	if !bsonDEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTranslateFilter_And(t *testing.T) {
	expr := parseFilter(t, `userName eq "alice" and active eq true`)
	got, err := TranslateFilter(expr)
	if err != nil {
		t.Fatalf("TranslateFilter: %v", err)
	}
	if len(got) != 1 || got[0].Key != "$and" {
		t.Errorf("expected $and operator, got %v", got)
	}
	arr, ok := got[0].Value.(bson.A)
	if !ok || len(arr) != 2 {
		t.Errorf("expected $and array with 2 elements, got %v", got[0].Value)
	}
}

func TestTranslateFilter_Or(t *testing.T) {
	expr := parseFilter(t, `userName eq "alice" or userName eq "bob"`)
	got, err := TranslateFilter(expr)
	if err != nil {
		t.Fatalf("TranslateFilter: %v", err)
	}
	if len(got) != 1 || got[0].Key != "$or" {
		t.Errorf("expected $or operator, got %v", got)
	}
	arr, ok := got[0].Value.(bson.A)
	if !ok || len(arr) != 2 {
		t.Errorf("expected $or array with 2 elements, got %v", got[0].Value)
	}
}

func TestTranslateFilter_Not(t *testing.T) {
	expr := parseFilter(t, `not (active eq false)`)
	got, err := TranslateFilter(expr)
	if err != nil {
		t.Fatalf("TranslateFilter: %v", err)
	}
	if len(got) != 1 || got[0].Key != "$nor" {
		t.Errorf("expected $nor operator, got %v", got)
	}
	arr, ok := got[0].Value.(bson.A)
	if !ok || len(arr) != 1 {
		t.Errorf("expected $nor array with 1 element, got %v", got[0].Value)
	}
}

func TestTranslateFilter_ValuePath(t *testing.T) {
	expr := parseFilter(t, `emails[type eq "work"]`)
	got, err := TranslateFilter(expr)
	if err != nil {
		t.Fatalf("TranslateFilter: %v", err)
	}
	if len(got) != 1 || got[0].Key != "emails" {
		t.Errorf("expected 'emails' field, got %v", got)
	}
	inner, ok := got[0].Value.(bson.D)
	if !ok || len(inner) != 1 || inner[0].Key != "$elemMatch" {
		t.Errorf("expected $elemMatch inside emails, got %v", got[0].Value)
	}
}

func TestTranslateFilter_Nil(t *testing.T) {
	got, err := TranslateFilter(nil)
	if err != nil {
		t.Fatalf("TranslateFilter(nil): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty bson.D for nil filter, got %v", got)
	}
}

func bsonDEqual(a, b bson.D) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key {
			return false
		}

		if fmt.Sprintf("%v", a[i].Value) != fmt.Sprintf("%v", b[i].Value) {
			return false
		}
	}
	return true
}
