package outbox

import (
	"context"
	"encoding/json"
	"fmt"

	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	domainmonitoring "github.com/remihneppo/be-go-template/internal/domain/monitoring"
	platformoutbox "github.com/remihneppo/be-go-template/internal/platform/outbox"
)

type Handler struct {
	auditLogs    domainauth.AuditLogRepository
	loginHistory domainauth.LoginHistoryRepository
	errorEvents  domainmonitoring.ErrorEventRepository
}

func NewHandler(auditLogs domainauth.AuditLogRepository, loginHistory domainauth.LoginHistoryRepository, errorEvents domainmonitoring.ErrorEventRepository) *Handler {
	return &Handler{auditLogs: auditLogs, loginHistory: loginHistory, errorEvents: errorEvents}
}

func (h *Handler) Handle(ctx context.Context, event platformoutbox.Event) error {
	switch event.Type {
	case TypeAuditLog:
		if h.auditLogs == nil {
			return fmt.Errorf("audit log repository is required")
		}
		var payload domainauth.AuditLog
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		return h.auditLogs.Append(ctx, payload)
	case TypeErrorEvent:
		if h.errorEvents == nil {
			return fmt.Errorf("error event repository is required")
		}
		var payload domainauth.ErrorEvent
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		return h.errorEvents.Append(ctx, payload)
	case TypeLoginHistory:
		if h.loginHistory == nil {
			return fmt.Errorf("login history repository is required")
		}
		var payload domainauth.LoginHistory
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		return h.loginHistory.Append(ctx, payload)
	default:
		return fmt.Errorf("unsupported outbox event type %q", event.Type)
	}
}
