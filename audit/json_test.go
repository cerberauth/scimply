package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNoopLogger(t *testing.T) {
	l := Noop()
	err := l.Log(context.Background(), Event{
		Timestamp: time.Now(),
		Operation: OperationCreate,
	})
	if err != nil {
		t.Fatalf("noop logger returned error: %v", err)
	}
}

func TestJSONLogger_SingleEvent(t *testing.T) {
	var buf bytes.Buffer
	l := NewJSONLogger(&buf)

	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	event := Event{
		Timestamp:    now,
		Operation:    OperationCreate,
		ResourceType: "User",
		ResourceID:   "abc-123",
		ActorID:      "admin",
		StatusCode:   201,
	}

	if err := l.Log(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected non-empty output")
	}

	var got Event
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	if got.Operation != OperationCreate {
		t.Errorf("operation: got %q, want %q", got.Operation, OperationCreate)
	}
	if got.ResourceType != "User" {
		t.Errorf("resourceType: got %q, want %q", got.ResourceType, "User")
	}
	if got.ResourceID != "abc-123" {
		t.Errorf("resourceId: got %q, want %q", got.ResourceID, "abc-123")
	}
	if got.StatusCode != 201 {
		t.Errorf("statusCode: got %d, want 201", got.StatusCode)
	}
}

func TestJSONLogger_MultipleEvents(t *testing.T) {
	var buf bytes.Buffer
	l := NewJSONLogger(&buf)

	ops := []Operation{OperationCreate, OperationRead, OperationDelete}
	for _, op := range ops {
		err := l.Log(context.Background(), Event{
			Timestamp: time.Now(),
			Operation: op,
		})
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", op, err)
		}
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	for i, line := range lines {
		var got Event
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d: failed to unmarshal: %v", i, err)
		}
		if got.Operation != ops[i] {
			t.Errorf("line %d: operation: got %q, want %q", i, got.Operation, ops[i])
		}
	}
}

func TestJSONLogger_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	l := NewJSONLogger(&buf)
	ctx := context.Background()

	const n = 100
	done := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_ = l.Log(ctx, Event{
				Timestamp: time.Now(),
				Operation: OperationCreate,
			})
		}()
	}
	for i := 0; i < n; i++ {
		<-done
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != n {
		t.Errorf("expected %d lines, got %d", n, len(lines))
	}
	for i, line := range lines {
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Errorf("line %d is invalid JSON: %v", i, err)
		}
	}
}

func TestJSONLogger_ErrorField(t *testing.T) {
	var buf bytes.Buffer
	l := NewJSONLogger(&buf)

	err := l.Log(context.Background(), Event{
		Timestamp:  time.Now(),
		Operation:  OperationDelete,
		StatusCode: 404,
		Error:      "resource not found",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got Event
	if unmarshalErr := json.Unmarshal(buf.Bytes(), &got); unmarshalErr != nil {
		t.Fatalf("failed to unmarshal: %v", unmarshalErr)
	}
	if got.Error != "resource not found" {
		t.Errorf("error field: got %q, want %q", got.Error, "resource not found")
	}
}
