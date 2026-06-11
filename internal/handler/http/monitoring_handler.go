package http

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	"github.com/remihneppo/be-go-template/internal/domain/monitoring"
)

type MonitoringHandler struct {
	service monitoring.Service
}

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
	events, err := h.service.GetRecentErrors(c.Request.Context(), queryPagination(c))
	if err != nil {
		reportContextError(c, err)
		return
	}
	OK(c, events)
}

func (h *MonitoringHandler) AuditLogs(c *gin.Context) {
	logs, err := h.service.GetRecentAuditLogs(c.Request.Context(), queryPagination(c))
	if err != nil {
		reportContextError(c, err)
		return
	}
	OK(c, logs)
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
