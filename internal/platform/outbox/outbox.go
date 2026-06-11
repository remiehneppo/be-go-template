package outbox

import (
	"context"
	"time"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusDone       Status = "done"
	StatusFailed     Status = "failed"
)

type Event struct {
	ID             string
	IdempotencyKey string
	Type           string
	Payload        []byte
	MaxRetries     int
	RetryCount     int
	Status         Status
	LastError      string
	ProcessAfter   time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Outbox interface {
	Enqueue(ctx context.Context, event Event) error
	ClaimBatch(ctx context.Context, limit int) ([]Event, error)
	MarkDone(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, reason string) error
}
