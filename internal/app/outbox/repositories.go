package outbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	domainmonitoring "github.com/remihneppo/be-go-template/internal/domain/monitoring"
	platformoutbox "github.com/remihneppo/be-go-template/internal/platform/outbox"
)

const (
	TypeAuditLog   = "audit_log"
	TypeErrorEvent = "error_event"
)

type AuditLogRepository struct {
	inner  domainauth.AuditLogRepository
	outbox platformoutbox.Outbox
	now    func() time.Time
}

func NewAuditLogRepository(inner domainauth.AuditLogRepository, outbox platformoutbox.Outbox) *AuditLogRepository {
	return &AuditLogRepository{
		inner:  inner,
		outbox: outbox,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

func (r *AuditLogRepository) Append(ctx context.Context, event domainauth.AuditLog) error {
	if r.outbox == nil {
		return r.inner.Append(ctx, event)
	}
	if event.ID == "" {
		event.ID = randomID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = r.now()
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return r.outbox.Enqueue(ctx, platformoutbox.Event{
		ID:             randomID(),
		IdempotencyKey: "audit_log:" + event.ID,
		Type:           TypeAuditLog,
		Payload:        payload,
	})
}

func (r *AuditLogRepository) List(ctx context.Context, filter domainauth.AuditLogFilter, pagination common.Pagination) ([]domainauth.AuditLog, error) {
	return r.inner.List(ctx, filter, pagination)
}

type ErrorEventRepository struct {
	inner  domainmonitoring.ErrorEventRepository
	outbox platformoutbox.Outbox
	now    func() time.Time
}

func NewErrorEventRepository(inner domainmonitoring.ErrorEventRepository, outbox platformoutbox.Outbox) *ErrorEventRepository {
	return &ErrorEventRepository{
		inner:  inner,
		outbox: outbox,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

func (r *ErrorEventRepository) Append(ctx context.Context, event domainauth.ErrorEvent) error {
	if r.outbox == nil {
		return r.inner.Append(ctx, event)
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = r.now()
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return r.outbox.Enqueue(ctx, platformoutbox.Event{
		ID:             randomID(),
		IdempotencyKey: errorEventIdempotencyKey(event),
		Type:           TypeErrorEvent,
		Payload:        payload,
	})
}

func (r *ErrorEventRepository) List(ctx context.Context, filter domainauth.ErrorEventFilter, pagination common.Pagination) ([]domainauth.ErrorEvent, error) {
	return r.inner.List(ctx, filter, pagination)
}

func errorEventIdempotencyKey(event domainauth.ErrorEvent) string {
	return fmt.Sprintf("error_event:%s:%s:%s:%s:%s:%s",
		event.RequestID,
		event.ErrorCode,
		event.Method,
		event.Path,
		strconv.Itoa(event.Status),
		event.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

var _ domainauth.AuditLogRepository = (*AuditLogRepository)(nil)
var _ domainmonitoring.ErrorEventRepository = (*ErrorEventRepository)(nil)
