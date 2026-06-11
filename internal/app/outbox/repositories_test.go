package outbox

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	platformoutbox "github.com/remihneppo/be-go-template/internal/platform/outbox"
)

func TestAuditLogRepositoryEnqueuesEvent(t *testing.T) {
	out := &fakeOutbox{}
	repo := NewAuditLogRepository(&fakeAuditLogRepository{}, out)
	repo.now = func() time.Time { return time.Unix(10, 0).UTC() }

	if err := repo.Append(context.Background(), domainauth.AuditLog{Action: "auth.login"}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if len(out.events) != 1 {
		t.Fatalf("events len = %d", len(out.events))
	}
	event := out.events[0]
	if event.Type != TypeAuditLog || event.IdempotencyKey == "" {
		t.Fatalf("event = %+v", event)
	}
	var payload domainauth.AuditLog
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("payload decode: %v", err)
	}
	if payload.ID == "" || payload.Action != "auth.login" || !payload.CreatedAt.Equal(time.Unix(10, 0).UTC()) {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestErrorEventRepositoryEnqueuesEvent(t *testing.T) {
	out := &fakeOutbox{}
	repo := NewErrorEventRepository(&fakeErrorEventRepository{}, out)
	repo.now = func() time.Time { return time.Unix(10, 0).UTC() }

	if err := repo.Append(context.Background(), domainauth.ErrorEvent{RequestID: "req-1", ErrorCode: "INTERNAL_ERROR", Status: 500}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if len(out.events) != 1 {
		t.Fatalf("events len = %d", len(out.events))
	}
	if out.events[0].Type != TypeErrorEvent || out.events[0].IdempotencyKey == "" {
		t.Fatalf("event = %+v", out.events[0])
	}
}

func TestHandlerRoutesEvents(t *testing.T) {
	auditRepo := &fakeAuditLogRepository{}
	errorRepo := &fakeErrorEventRepository{}
	handler := NewHandler(auditRepo, errorRepo)
	auditPayload, _ := json.Marshal(domainauth.AuditLog{ID: "a1", Action: "auth.login"})
	errorPayload, _ := json.Marshal(domainauth.ErrorEvent{RequestID: "req-1", ErrorCode: "INTERNAL_ERROR"})

	if err := handler.Handle(context.Background(), platformoutbox.Event{Type: TypeAuditLog, Payload: auditPayload}); err != nil {
		t.Fatalf("Handle(audit) error = %v", err)
	}
	if err := handler.Handle(context.Background(), platformoutbox.Event{Type: TypeErrorEvent, Payload: errorPayload}); err != nil {
		t.Fatalf("Handle(error) error = %v", err)
	}
	if len(auditRepo.events) != 1 || auditRepo.events[0].Action != "auth.login" {
		t.Fatalf("audit events = %+v", auditRepo.events)
	}
	if len(errorRepo.events) != 1 || errorRepo.events[0].RequestID != "req-1" {
		t.Fatalf("error events = %+v", errorRepo.events)
	}
}

type fakeOutbox struct {
	events []platformoutbox.Event
}

func (o *fakeOutbox) Enqueue(ctx context.Context, event platformoutbox.Event) error {
	o.events = append(o.events, event)
	return nil
}

func (o *fakeOutbox) ClaimBatch(ctx context.Context, limit int) ([]platformoutbox.Event, error) {
	return nil, nil
}

func (o *fakeOutbox) MarkDone(ctx context.Context, id string) error {
	return nil
}

func (o *fakeOutbox) MarkFailed(ctx context.Context, id string, reason string) error {
	return nil
}

type fakeAuditLogRepository struct {
	events []domainauth.AuditLog
}

func (r *fakeAuditLogRepository) Append(ctx context.Context, event domainauth.AuditLog) error {
	r.events = append(r.events, event)
	return nil
}

func (r *fakeAuditLogRepository) List(ctx context.Context, filter domainauth.AuditLogFilter, pagination common.Pagination) ([]domainauth.AuditLog, error) {
	return r.events, nil
}

type fakeErrorEventRepository struct {
	events []domainauth.ErrorEvent
}

func (r *fakeErrorEventRepository) Append(ctx context.Context, event domainauth.ErrorEvent) error {
	r.events = append(r.events, event)
	return nil
}

func (r *fakeErrorEventRepository) List(ctx context.Context, pagination common.Pagination) ([]domainauth.ErrorEvent, error) {
	return r.events, nil
}
