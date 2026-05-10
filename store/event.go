package store

import (
	"time"

	"github.com/cerberauth/scimply/resource"
)

type EventType string

const (
	EventCreated EventType = "created"
	EventUpdated EventType = "updated"
	EventDeleted EventType = "deleted"
)

type Event struct {
	Type         EventType
	ResourceType string
	ResourceID   string
	Resource     *resource.Resource
	Timestamp    time.Time
}
