package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	domainmonitoring "github.com/remihneppo/be-go-template/internal/domain/monitoring"
	"github.com/remihneppo/be-go-template/internal/platform/database"
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
		return ignoreConflict(h.auditLogs.Append(ctx, payload))
	case TypeErrorEvent:
		if h.errorEvents == nil {
			return fmt.Errorf("error event repository is required")
		}
		var payload domainauth.ErrorEvent
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		return ignoreConflict(h.errorEvents.Append(ctx, payload))
	case TypeLoginHistory:
		if h.loginHistory == nil {
			return fmt.Errorf("login history repository is required")
		}
		var payload domainauth.LoginHistory
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		return ignoreConflict(h.loginHistory.Append(ctx, payload))
	default:
		return fmt.Errorf("unsupported outbox event type %q", event.Type)
	}
}

func ignoreConflict(err error) error {
	if err == nil || !errors.Is(err, database.ErrConflict) {
		return err
	}
	return nil
}
