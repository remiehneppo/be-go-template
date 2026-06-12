package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/config"
	"github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	domainmonitoring "github.com/remihneppo/be-go-template/internal/domain/monitoring"
	"github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

func TestMonitoringRoutesReturnPayloadsAndQueryParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &monitoringServiceStub{
		status: &domainmonitoring.SystemStatus{
			Status:        domainmonitoring.Degraded,
			ServiceName:   "api",
			Version:       "test",
			Env:           "production",
			StartedAt:     time.Unix(1, 0).UTC(),
			UptimeSeconds: 9,
			CheckedAt:     time.Unix(10, 0).UTC(),
		},
		dependencies: &domainmonitoring.DependencyStatus{
			MongoDB: domainmonitoring.DependencyCheck{Status: domainmonitoring.Healthy},
			Redis:   domainmonitoring.DependencyCheck{Status: domainmonitoring.Unhealthy, Error: "redis down"},
		},
		runtime: &domainmonitoring.RuntimeMetrics{Goroutines: 4, CollectedAt: time.Unix(11, 0).UTC()},
		authStats: &domainmonitoring.AuthStats{
			LoginSuccessCount: 3,
			From:              time.Unix(100, 0).UTC(),
			To:                time.Unix(200, 0).UTC(),
		},
		errors: []auth.ErrorEvent{{RequestID: "req-1", ErrorCode: "INTERNAL_ERROR", Operation: "AuthService.Refresh", Message: "safe message", Cause: "db password leaked", Stack: "panic stack", Path: "/v1/auth/refresh", Method: http.MethodPost, Status: http.StatusInternalServerError, UserID: "u1", CreatedAt: time.Unix(12, 0).UTC()}},
		audits: []auth.AuditLog{{ID: "a1", Action: "auth.login", Metadata: map[string]string{"refresh_token": "secret-refresh", "email": "admin@example.com"}}},
	}
	router := NewRouterWithDependencies(testConfig(), logger.NewNoop(), RouterDependencies{
		Monitoring: service,
		TokenService: &fakeHTTPTokenService{claims: &auth.AccessClaims{
			UserID:    "admin-1",
			SessionID: "s1",
			TokenID:   "jti1",
			Roles:     []string{string(user.RoleAdmin)},
		}},
	})

	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{name: "status", method: http.MethodGet, path: "/v1/admin/monitoring/status", want: `"env":"production"`},
		{name: "dependencies", method: http.MethodGet, path: "/v1/admin/monitoring/dependencies", want: `"redis down"`},
		{name: "runtime", method: http.MethodGet, path: "/v1/admin/monitoring/runtime", want: `"goroutines":4`},
		{name: "auth stats", method: http.MethodGet, path: "/v1/admin/monitoring/auth-stats?from=1970-01-01T00:01:40Z&to=1970-01-01T00:03:20Z", want: `"login_success_count":3`},
		{name: "errors", method: http.MethodGet, path: "/v1/admin/monitoring/errors?limit=7&offset=2&cursor=abc&error_code=INTERNAL_ERROR&request_id=req-1&operation=AuthService.Refresh&status=500", want: `AuthService.Refresh`},
		{name: "audit logs", method: http.MethodGet, path: "/v1/admin/monitoring/audit-logs?limit=5&offset=1&actor_user_id=admin-1&action=auth.login&resource_type=session&resource_id=s1", want: `auth.login`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Authorization", "Bearer access-token")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("body = %s", rec.Body.String())
			}
		})
	}

	if service.authFrom != time.Unix(100, 0).UTC() || service.authTo != time.Unix(200, 0).UTC() {
		t.Fatalf("auth range = %s %s", service.authFrom, service.authTo)
	}
	if service.errorsPagination.Limit != 7 || service.errorsPagination.Offset != 2 || service.errorsPagination.Cursor != "abc" {
		t.Fatalf("errors pagination = %+v", service.errorsPagination)
	}
	if service.errorsFilter.Operation != "AuthService.Refresh" || service.errorsFilter.ErrorCode != "INTERNAL_ERROR" || service.errorsFilter.RequestID != "req-1" {
		t.Fatalf("errors filter = %+v", service.errorsFilter)
	}
	if service.auditPagination.Limit != 5 || service.auditPagination.Offset != 1 {
		t.Fatalf("audit pagination = %+v", service.auditPagination)
	}
	if service.errorsFilter.ErrorCode != "INTERNAL_ERROR" || service.errorsFilter.RequestID != "req-1" || service.errorsFilter.Status != 500 {
		t.Fatalf("errors filter = %+v", service.errorsFilter)
	}
	if service.auditFilter.ActorUserID != "admin-1" || service.auditFilter.Action != "auth.login" || service.auditFilter.ResourceType != "session" || service.auditFilter.ResourceID != "s1" {
		t.Fatalf("audit filter = %+v", service.auditFilter)
	}

	errorsReq := httptest.NewRequest(http.MethodGet, "/v1/admin/monitoring/errors?request_id=req-1", nil)
	errorsReq.Header.Set("Authorization", "Bearer access-token")
	errorsRec := httptest.NewRecorder()
	router.ServeHTTP(errorsRec, errorsReq)
	if strings.Contains(errorsRec.Body.String(), "db password leaked") || strings.Contains(errorsRec.Body.String(), "panic stack") || strings.Contains(errorsRec.Body.String(), `"cause"`) || strings.Contains(errorsRec.Body.String(), `"stack"`) {
		t.Fatalf("errors body leaked sensitive data: %s", errorsRec.Body.String())
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/v1/admin/monitoring/audit-logs?actor_user_id=admin-1", nil)
	auditReq.Header.Set("Authorization", "Bearer access-token")
	auditRec := httptest.NewRecorder()
	router.ServeHTTP(auditRec, auditReq)
	if !strings.Contains(auditRec.Body.String(), `"[REDACTED]"`) || strings.Contains(auditRec.Body.String(), "secret-refresh") {
		t.Fatalf("audit body leaked sensitive data: %s", auditRec.Body.String())
	}
}

func TestMonitoringRoutesDisabledByConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Config{}
	cfg.App.Name = "test"
	cfg.HTTP.CORSAllowOrigins = []string{"http://localhost:3000"}
	cfg.Monitoring.Enabled = false

	router := NewRouterWithDependencies(cfg, logger.NewNoop(), RouterDependencies{
		Monitoring: &monitoringServiceStub{},
		TokenService: &fakeHTTPTokenService{claims: &auth.AccessClaims{
			UserID:    "admin-1",
			SessionID: "s1",
			TokenID:   "jti1",
			Roles:     []string{string(user.RoleAdmin)},
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/monitoring/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

type monitoringServiceStub struct {
	status           *domainmonitoring.SystemStatus
	dependencies     *domainmonitoring.DependencyStatus
	runtime          *domainmonitoring.RuntimeMetrics
	authStats        *domainmonitoring.AuthStats
	errors           []auth.ErrorEvent
	audits           []auth.AuditLog
	authFrom         time.Time
	authTo           time.Time
	errorsFilter     auth.ErrorEventFilter
	auditFilter      auth.AuditLogFilter
	errorsPagination common.Pagination
	auditPagination  common.Pagination
}

func (s *monitoringServiceStub) GetSystemStatus(ctx context.Context) (*domainmonitoring.SystemStatus, error) {
	return s.status, nil
}

func (s *monitoringServiceStub) GetRuntimeMetrics(ctx context.Context) (*domainmonitoring.RuntimeMetrics, error) {
	return s.runtime, nil
}

func (s *monitoringServiceStub) GetDependencyStatus(ctx context.Context) (*domainmonitoring.DependencyStatus, error) {
	return s.dependencies, nil
}

func (s *monitoringServiceStub) GetAuthStats(ctx context.Context, from time.Time, to time.Time) (*domainmonitoring.AuthStats, error) {
	s.authFrom = from
	s.authTo = to
	return s.authStats, nil
}

func (s *monitoringServiceStub) GetRecentErrors(ctx context.Context, filter auth.ErrorEventFilter, pagination common.Pagination) ([]auth.ErrorEvent, error) {
	s.errorsFilter = filter
	s.errorsPagination = pagination
	return s.errors, nil
}

func (s *monitoringServiceStub) GetRecentAuditLogs(ctx context.Context, filter auth.AuditLogFilter, pagination common.Pagination) ([]auth.AuditLog, error) {
	s.auditFilter = filter
	s.auditPagination = pagination
	return s.audits, nil
}

var _ domainmonitoring.Service = (*monitoringServiceStub)(nil)
