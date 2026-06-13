package http

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	"github.com/remihneppo/be-go-template/internal/domain/monitoring"
)

// MonitoringHandler handles HTTP requests for admin monitoring endpoints.
type MonitoringHandler struct {
	service monitoring.Service
}

// NewMonitoringHandler creates a MonitoringHandler from the given service.
func NewMonitoringHandler(service monitoring.Service) *MonitoringHandler {
	return &MonitoringHandler{service: service}
}

func (h *MonitoringHandler) RegisterRoutes(group *gin.RouterGroup) {
	monitoringGroup := group.Group("/monitoring")
	monitoringGroup.GET("/status", h.SystemStatus)
	monitoringGroup.GET("/dependencies", h.Dependencies)
	monitoringGroup.GET("/runtime", h.Runtime)
	monitoringGroup.GET("/auth-stats", h.AuthStats)
	monitoringGroup.GET("/errors", h.Errors)
	monitoringGroup.GET("/audit-logs", h.AuditLogs)
}

func (h *MonitoringHandler) SystemStatus(c *gin.Context) {
	status, err := h.service.GetSystemStatus(c.Request.Context())
	if err != nil {
		reportContextError(c, err)
		return
	}
	OK(c, status)
}

func (h *MonitoringHandler) Dependencies(c *gin.Context) {
	status, err := h.service.GetDependencyStatus(c.Request.Context())
	if err != nil {
		reportContextError(c, err)
		return
	}
	OK(c, status)
}

func (h *MonitoringHandler) Runtime(c *gin.Context) {
	metrics, err := h.service.GetRuntimeMetrics(c.Request.Context())
	if err != nil {
		reportContextError(c, err)
		return
	}
	OK(c, metrics)
}

func (h *MonitoringHandler) AuthStats(c *gin.Context) {
	stats, err := h.service.GetAuthStats(c.Request.Context(), queryTime(c, "from"), queryTime(c, "to"))
	if err != nil {
		reportContextError(c, err)
		return
	}
	OK(c, stats)
}

func (h *MonitoringHandler) Errors(c *gin.Context) {
	events, err := h.service.GetRecentErrors(c.Request.Context(), queryErrorEventFilter(c), queryPagination(c))
	if err != nil {
		reportContextError(c, err)
		return
	}
	OK(c, sanitizeErrorEvents(events))
}

func (h *MonitoringHandler) AuditLogs(c *gin.Context) {
	logs, err := h.service.GetRecentAuditLogs(c.Request.Context(), queryAuditLogFilter(c), queryPagination(c))
	if err != nil {
		reportContextError(c, err)
		return
	}
	OK(c, sanitizeAuditLogs(logs))
}

func queryPagination(c *gin.Context) common.Pagination {
	return common.Pagination{
		Limit:  queryInt(c, "limit"),
		Offset: queryInt(c, "offset"),
		Cursor: c.Query("cursor"),
	}
}

func queryInt(c *gin.Context, key string) int {
	value, err := strconv.Atoi(c.Query(key))
	if err != nil {
		return 0
	}
	return value
}

func queryTime(c *gin.Context, key string) time.Time {
	value := c.Query(key)
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func queryErrorEventFilter(c *gin.Context) auth.ErrorEventFilter {
	status := queryInt(c, "status")
	return auth.ErrorEventFilter{
		ErrorCode: c.Query("error_code"),
		RequestID: c.Query("request_id"),
		Operation: c.Query("operation"),
		Status:    status,
		From:      queryTime(c, "from"),
		To:        queryTime(c, "to"),
	}
}

func queryAuditLogFilter(c *gin.Context) auth.AuditLogFilter {
	return auth.AuditLogFilter{
		ActorUserID:  c.Query("actor_user_id"),
		Action:       c.Query("action"),
		ResourceType: c.Query("resource_type"),
		ResourceID:   c.Query("resource_id"),
		RequestID:    c.Query("request_id"),
		From:         queryTime(c, "from"),
		To:           queryTime(c, "to"),
	}
}

type monitoringErrorEventView struct {
	RequestID string    `json:"request_id"`
	ErrorCode string    `json:"error_code"`
	Operation string    `json:"operation,omitempty"`
	Message   string    `json:"message"`
	Path      string    `json:"path"`
	Method    string    `json:"method"`
	Status    int       `json:"status"`
	UserID    string    `json:"user_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type monitoringAuditLogView struct {
	ID           string            `json:"id"`
	ActorUserID  string            `json:"actor_user_id"`
	Action       string            `json:"action"`
	ResourceType string            `json:"resource_type"`
	ResourceID   string            `json:"resource_id"`
	IP           string            `json:"ip"`
	UserAgent    string            `json:"user_agent"`
	RequestID    string            `json:"request_id"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
}

func sanitizeErrorEvents(events []auth.ErrorEvent) []monitoringErrorEventView {
	views := make([]monitoringErrorEventView, 0, len(events))
	for _, event := range events {
		views = append(views, monitoringErrorEventView{
			RequestID: event.RequestID,
			ErrorCode: event.ErrorCode,
			Operation: event.Operation,
			Message:   event.Message,
			Path:      event.Path,
			Method:    event.Method,
			Status:    event.Status,
			UserID:    event.UserID,
			CreatedAt: event.CreatedAt,
		})
	}
	return views
}

func sanitizeAuditLogs(logs []auth.AuditLog) []monitoringAuditLogView {
	views := make([]monitoringAuditLogView, 0, len(logs))
	for _, log := range logs {
		views = append(views, monitoringAuditLogView{
			ID:           log.ID,
			ActorUserID:  log.ActorUserID,
			Action:       log.Action,
			ResourceType: log.ResourceType,
			ResourceID:   log.ResourceID,
			IP:           log.IP,
			UserAgent:    log.UserAgent,
			RequestID:    log.RequestID,
			Metadata:     redactMetadata(log.Metadata),
			CreatedAt:    log.CreatedAt,
		})
	}
	return views
}

func redactMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	redacted := make(map[string]string, len(metadata))
	for key, value := range metadata {
		if isSensitiveField(key) {
			redacted[key] = "[REDACTED]"
			continue
		}
		redacted[key] = value
	}
	return redacted
}

func isSensitiveField(key string) bool {
	normalized := strings.ToLower(key)
	for _, marker := range []string{"password", "token", "secret", "authorization", "refresh"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
