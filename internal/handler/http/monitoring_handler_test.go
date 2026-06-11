package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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
			Status:      domainmonitoring.Degraded,
			ServiceName: "api",
			Version:     "test",
			CheckedAt:   time.Unix(10, 0).UTC(),
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
		errors: []auth.ErrorEvent{{RequestID: "req-1", ErrorCode: "INTERNAL_ERROR"}},
		audits: []auth.AuditLog{{ID: "a1", Action: "auth.login"}},
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
		{name: "status", method: http.MethodGet, path: "/v1/admin/monitoring/status", want: `"service_name":"api"`},
		{name: "dependencies", method: http.MethodGet, path: "/v1/admin/monitoring/dependencies", want: `"redis down"`},
		{name: "runtime", method: http.MethodGet, path: "/v1/admin/monitoring/runtime", want: `"goroutines":4`},
		{name: "auth stats", method: http.MethodGet, path: "/v1/admin/monitoring/auth-stats?from=1970-01-01T00:01:40Z&to=1970-01-01T00:03:20Z", want: `"login_success_count":3`},
		{name: "errors", method: http.MethodGet, path: "/v1/admin/monitoring/errors?limit=7&offset=2&cursor=abc", want: `req-1`},
		{name: "audit logs", method: http.MethodGet, path: "/v1/admin/monitoring/audit-logs?limit=5&offset=1", want: `auth.login`},
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
	if service.auditPagination.Limit != 5 || service.auditPagination.Offset != 1 {
		t.Fatalf("audit pagination = %+v", service.auditPagination)
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

func (s *monitoringServiceStub) GetRecentErrors(ctx context.Context, pagination common.Pagination) ([]auth.ErrorEvent, error) {
	s.errorsPagination = pagination
	return s.errors, nil
}

func (s *monitoringServiceStub) GetRecentAuditLogs(ctx context.Context, pagination common.Pagination) ([]auth.AuditLog, error) {
	s.auditPagination = pagination
	return s.audits, nil
}

var _ domainmonitoring.Service = (*monitoringServiceStub)(nil)
