package audit

import (
	"context"
	"time"
)

type Operation string

const (
	OperationCreate  Operation = "create"
	OperationRead    Operation = "read"
	OperationReplace Operation = "replace"
	OperationPatch   Operation = "patch"
	OperationDelete  Operation = "delete"
	OperationList    Operation = "list"
	OperationBulk    Operation = "bulk"
)

type Event struct {
	Timestamp    time.Time `json:"timestamp"`
	Operation    Operation `json:"operation"`
	ResourceType string    `json:"resourceType,omitempty"`
	ResourceID   string    `json:"resourceId,omitempty"`
	ActorID      string    `json:"actorId,omitempty"`
	RequestID    string    `json:"requestId,omitempty"`
	StatusCode   int       `json:"statusCode"`
	Error        string    `json:"error,omitempty"`
	Attributes   []string  `json:"attributes,omitempty"`
}

type Logger interface {
	Log(ctx context.Context, event Event) error
}
