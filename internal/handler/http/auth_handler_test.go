package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/remihneppo/be-go-template/internal/config"
	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	"github.com/remihneppo/be-go-template/internal/domain/monitoring"
	"github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
	platformmetrics "github.com/remihneppo/be-go-template/internal/platform/metrics"
	"github.com/remihneppo/be-go-template/internal/platform/ratelimit"
)

func TestAuthHandlerLoginDoesNotExposePasswordHash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithDependencies(testConfig(), logger.NewNoop(), RouterDependencies{
		AuthService: &fakeAuthService{
			result: &domainauth.AuthResult{
				User: user.User{
					ID:           "u1",
					Email:        "user@example.com",
					PasswordHash: "secret-hash",
					Name:         "User",
					Roles:        []user.Role{user.RoleUser},
				},
				SessionID:             "s1",
				AccessToken:           "access",
				AccessTokenExpiresAt:  time.Unix(100, 0).UTC(),
				RefreshToken:          "refresh",
				RefreshTokenExpiresAt: time.Unix(200, 0).UTC(),
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"email":"user@example.com","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "secret-hash") || strings.Contains(body, "password_hash") {
		t.Fatalf("response leaked password hash: %s", body)
	}
	if !strings.Contains(body, `"access_token":"access"`) {
		t.Fatalf("response missing access token: %s", body)
	}
}

func TestAuthHandlerInvalidJSONReturnsValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithDependencies(testConfig(), logger.NewNoop(), RouterDependencies{AuthService: &fakeAuthService{}})

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req-123")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Success   bool   `json:"success"`
		RequestID string `json:"request_id"`
		Error     struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details []struct {
				Field  string                 `json:"field"`
				Reason string                 `json:"reason"`
				Meta   map[string]interface{} `json:"meta"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v body = %s", err, rec.Body.String())
	}
	if response.RequestID != "req-123" {
		t.Fatalf("request_id = %q body = %s", response.RequestID, rec.Body.String())
	}
	if got := rec.Header().Get("X-Request-ID"); got != "req-123" {
		t.Fatalf("X-Request-ID = %q body = %s", got, rec.Body.String())
	}
	if response.Success {
		t.Fatal("success = true")
	}
	if response.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("code = %q body = %s", response.Error.Code, rec.Body.String())
	}
	if response.Error.Message != "Invalid input" {
		t.Fatalf("message = %q body = %s", response.Error.Message, rec.Body.String())
	}
	if len(response.Error.Details) != 1 || response.Error.Details[0].Field != "body" || response.Error.Details[0].Reason != "invalid_json" {
		t.Fatalf("details = %+v body = %s", response.Error.Details, rec.Body.String())
	}
	if response.Error.Details[0].Meta["kind"] != "syntax" {
		t.Fatalf("meta = %+v body = %s", response.Error.Details[0].Meta, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "unexpected EOF") || strings.Contains(rec.Body.String(), "invalid character") {
		t.Fatalf("body leaked raw bind error: %s", rec.Body.String())
	}
}

func TestAuthHandlerLoginTypeMismatchReturnsStructuredValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithDependencies(testConfig(), logger.NewNoop(), RouterDependencies{AuthService: &fakeAuthService{}})

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"email":123,"password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details []struct {
				Field  string                 `json:"field"`
				Reason string                 `json:"reason"`
				Meta   map[string]interface{} `json:"meta"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v body = %s", err, rec.Body.String())
	}
	if response.Error.Code != "VALIDATION_ERROR" || response.Error.Message != "Invalid input" {
		t.Fatalf("response = %+v body = %s", response.Error, rec.Body.String())
	}
	if len(response.Error.Details) != 1 {
		t.Fatalf("details = %+v body = %s", response.Error.Details, rec.Body.String())
	}
	detail := response.Error.Details[0]
	if detail.Field != "body.email" || detail.Reason != "invalid_type" {
		t.Fatalf("detail = %+v body = %s", detail, rec.Body.String())
	}
	if detail.Meta["expected"] != "string" {
		t.Fatalf("meta = %+v body = %s", detail.Meta, rec.Body.String())
	}
}

func TestAuthHandlerLoginRateLimitReturns429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := testConfig()
	cfg.RateLimit.AuthEnabled = true
	cfg.RateLimit.LoginPerMinute = 1
	router := NewRouterWithDependencies(cfg, logger.NewNoop(), RouterDependencies{
		AuthService: &fakeAuthService{},
		RateLimiter: &fakeRouteLimiter{
			decision: ratelimit.Decision{Allowed: false, Limit: 1},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"email":"USER@example.com","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "RATE_LIMITED") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestAuthHandlerDevicesRequiresAccessToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authService := &fakeAuthService{}
	router := NewRouterWithDependencies(testConfig(), logger.NewNoop(), RouterDependencies{
		AuthService: authService,
		TokenService: &fakeHTTPTokenService{claims: &domainauth.AccessClaims{
			UserID:    "u1",
			SessionID: "s1",
			TokenID:   "jti1",
			Roles:     []string{"user"},
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/devices", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/auth/devices", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("authorized status = %d body = %s", rec.Code, rec.Body.String())
	}
	if authService.listDevicesUserID != "u1" {
		t.Fatalf("ListDevices user id = %q", authService.listDevicesUserID)
	}
}

func TestAuthHandlerDevicesReturnsNotModifiedForMatchingETag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithDependencies(testConfig(), logger.NewNoop(), RouterDependencies{
		AuthService: &fakeAuthService{},
		TokenService: &fakeHTTPTokenService{claims: &domainauth.AccessClaims{
			UserID:    "u1",
			SessionID: "s1",
			TokenID:   "jti1",
			Roles:     []string{"user"},
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/devices", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	etag := rec.Header().Get("ETag")
	if rec.Code != http.StatusOK || etag == "" {
		t.Fatalf("first status = %d etag = %q body = %s", rec.Code, etag, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/auth/devices", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Fatalf("second status = %d body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("304 body = %s", rec.Body.String())
	}
}

func TestAuthHandlerLoginHistoryUsesPaginationQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authService := &fakeAuthService{}
	router := NewRouterWithDependencies(testConfig(), logger.NewNoop(), RouterDependencies{
		AuthService: authService,
		TokenService: &fakeHTTPTokenService{claims: &domainauth.AccessClaims{
			UserID:    "u1",
			SessionID: "s1",
			TokenID:   "jti1",
			Roles:     []string{"user"},
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/login-history?limit=7&offset=3&cursor=abc", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if authService.listLoginHistoryUserID != "u1" {
		t.Fatalf("ListLoginHistory user id = %q", authService.listLoginHistoryUserID)
	}
	if authService.listLoginHistoryPagination.Limit != 7 || authService.listLoginHistoryPagination.Offset != 3 || authService.listLoginHistoryPagination.Cursor != "abc" {
		t.Fatalf("pagination = %+v", authService.listLoginHistoryPagination)
	}
}

func TestUserMeReturnsProfileWithETag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userService := &fakeUserService{user: &user.User{ID: "u1", Email: "user@example.com", Name: "User", Roles: []user.Role{user.RoleUser}}}
	router := NewRouterWithDependencies(testConfig(), logger.NewNoop(), RouterDependencies{
		UserService: userService,
		TokenService: &fakeHTTPTokenService{claims: &domainauth.AccessClaims{
			UserID:    "u1",
			SessionID: "s1",
			TokenID:   "jti1",
			Roles:     []string{"user"},
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	etag := rec.Header().Get("ETag")

	if rec.Code != http.StatusOK || etag == "" {
		t.Fatalf("status = %d etag = %q body = %s", rec.Code, etag, rec.Body.String())
	}
	if userService.userID != "u1" {
		t.Fatalf("GetMe user id = %q", userService.userID)
	}
	if strings.Contains(rec.Body.String(), "password_hash") {
		t.Fatalf("response leaked password field: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("second status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestRouterMetricsEndpointUsesPrometheusFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := testConfig()
	registry := prometheus.NewRegistry()
	httpMetrics, err := platformmetrics.NewHTTPMetrics(registry, "testapp")
	if err != nil {
		t.Fatalf("NewHTTPMetrics() error = %v", err)
	}
	router := NewRouterWithDependencies(cfg, logger.NewNoop(), RouterDependencies{
		HTTPMetrics:    httpMetrics,
		MetricsHandler: gin.WrapH(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})),
	})

	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthRec := httptest.NewRecorder()
	router.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("health status = %d", healthRec.Code)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	router.ServeHTTP(metricsRec, metricsReq)

	if metricsRec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d body = %s", metricsRec.Code, metricsRec.Body.String())
	}
	body := metricsRec.Body.String()
	if !strings.Contains(body, "testapp_http_requests_total") || !strings.Contains(body, "testapp_http_request_duration_seconds") || !strings.Contains(body, `route="/healthz"`) {
		t.Fatalf("metrics body = %s", body)
	}
}

func TestRouterReadyzReturnsDependencyStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithDependencies(testConfig(), logger.NewNoop(), RouterDependencies{
		Readiness: &fakeReadiness{ready: true, status: monitoring.DependencyStatus{
			MongoDB: monitoring.DependencyCheck{Status: monitoring.Healthy},
			Redis:   monitoring.DependencyCheck{Status: monitoring.Unhealthy, Error: "redis down"},
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"status":"degraded"`) || !strings.Contains(body, `"redis"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestRouterReadyzReturns503WhenNotReady(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithDependencies(testConfig(), logger.NewNoop(), RouterDependencies{
		Readiness: &fakeReadiness{ready: false, status: monitoring.DependencyStatus{
			MongoDB: monitoring.DependencyCheck{Status: monitoring.Unhealthy, Error: "mongo down"},
			Redis:   monitoring.DependencyCheck{Status: monitoring.Healthy},
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"unhealthy"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestRouterHealthzAndReadyzReturnSuccessWhenReady(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithDependencies(testConfig(), logger.NewNoop(), RouterDependencies{
		Readiness: &fakeReadiness{ready: true, status: monitoring.DependencyStatus{
			MongoDB: monitoring.DependencyCheck{Status: monitoring.Healthy},
			Redis:   monitoring.DependencyCheck{Status: monitoring.Healthy},
		}},
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	healthResp, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatalf("healthz request error = %v", err)
	}
	defer healthResp.Body.Close()
	healthBody, err := io.ReadAll(healthResp.Body)
	if err != nil {
		t.Fatalf("healthz read error = %v", err)
	}
	if healthResp.StatusCode != http.StatusOK || !strings.Contains(string(healthBody), `"success":true`) || !strings.Contains(string(healthBody), `"data"`) {
		t.Fatalf("healthz = %d body = %s", healthResp.StatusCode, string(healthBody))
	}

	readyResp, err := http.Get(server.URL + "/readyz")
	if err != nil {
		t.Fatalf("readyz request error = %v", err)
	}
	defer readyResp.Body.Close()
	readyBody, err := io.ReadAll(readyResp.Body)
	if err != nil {
		t.Fatalf("readyz read error = %v", err)
	}
	if readyResp.StatusCode != http.StatusOK || !strings.Contains(string(readyBody), `"success":true`) || !strings.Contains(string(readyBody), `"status":"healthy"`) {
		t.Fatalf("readyz = %d body = %s", readyResp.StatusCode, string(readyBody))
	}
}

func TestAdminMonitoringRequiresAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithDependencies(testConfig(), logger.NewNoop(), RouterDependencies{
		Monitoring: &fakeMonitoringService{status: &monitoring.SystemStatus{Status: monitoring.Healthy, ServiceName: "api", Version: "test"}},
		TokenService: &fakeHTTPTokenService{claims: &domainauth.AccessClaims{
			UserID:    "u1",
			SessionID: "s1",
			TokenID:   "jti1",
			Roles:     []string{"user"},
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/monitoring/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/admin/monitoring/status", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user role status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminMonitoringStatusAllowsAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithDependencies(testConfig(), logger.NewNoop(), RouterDependencies{
		Monitoring: &fakeMonitoringService{status: &monitoring.SystemStatus{Status: monitoring.Healthy, ServiceName: "api", Version: "test"}},
		TokenService: &fakeHTTPTokenService{claims: &domainauth.AccessClaims{
			UserID:    "admin1",
			SessionID: "s1",
			TokenID:   "jti1",
			Roles:     []string{"admin"},
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/monitoring/status", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"service_name":"api"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func testConfig() config.Config {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	return cfg
}

type fakeAuthService struct {
	result                     *domainauth.AuthResult
	listDevicesUserID          string
	listLoginHistoryUserID     string
	listLoginHistoryPagination common.Pagination
}

func (s *fakeAuthService) Register(ctx context.Context, input domainauth.RegisterInput) (*domainauth.AuthResult, error) {
	return s.result, nil
}

func (s *fakeAuthService) Login(ctx context.Context, input domainauth.LoginInput, meta domainauth.RequestMeta) (*domainauth.AuthResult, error) {
	return s.result, nil
}

func (s *fakeAuthService) Refresh(ctx context.Context, refreshToken string, meta domainauth.RequestMeta) (*domainauth.AuthResult, error) {
	return s.result, nil
}

func (s *fakeAuthService) Logout(ctx context.Context, accessToken string, sessionID string) error {
	return nil
}

func (s *fakeAuthService) LogoutAll(ctx context.Context, userID string) error {
	return nil
}

func (s *fakeAuthService) ListDevices(ctx context.Context, userID string) ([]domainauth.DeviceSession, error) {
	s.listDevicesUserID = userID
	return []domainauth.DeviceSession{{SessionID: "s1", DeviceID: "d1"}}, nil
}

func (s *fakeAuthService) ListLoginHistory(ctx context.Context, userID string, pagination common.Pagination) ([]domainauth.LoginHistory, error) {
	s.listLoginHistoryUserID = userID
	s.listLoginHistoryPagination = pagination
	return nil, nil
}

type fakeRouteLimiter struct {
	decision ratelimit.Decision
}

func (l *fakeRouteLimiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (ratelimit.Decision, error) {
	return l.decision, nil
}

type fakeUserService struct {
	user   *user.User
	userID string
}

func (s *fakeUserService) GetMe(ctx context.Context, userID string) (*user.User, error) {
	s.userID = userID
	return s.user, nil
}

type fakeReadiness struct {
	status monitoring.DependencyStatus
	ready  bool
}

func (r *fakeReadiness) Check(ctx context.Context) (monitoring.DependencyStatus, bool) {
	return r.status, r.ready
}

type fakeMonitoringService struct {
	status *monitoring.SystemStatus
}

func (s *fakeMonitoringService) GetSystemStatus(ctx context.Context) (*monitoring.SystemStatus, error) {
	return s.status, nil
}

func (s *fakeMonitoringService) GetRuntimeMetrics(ctx context.Context) (*monitoring.RuntimeMetrics, error) {
	return &monitoring.RuntimeMetrics{}, nil
}

func (s *fakeMonitoringService) GetDependencyStatus(ctx context.Context) (*monitoring.DependencyStatus, error) {
	return &monitoring.DependencyStatus{}, nil
}

func (s *fakeMonitoringService) GetAuthStats(ctx context.Context, from time.Time, to time.Time) (*monitoring.AuthStats, error) {
	return &monitoring.AuthStats{From: from, To: to}, nil
}

func (s *fakeMonitoringService) GetRecentErrors(ctx context.Context, filter domainauth.ErrorEventFilter, pagination common.Pagination) ([]domainauth.ErrorEvent, error) {
	return nil, nil
}

func (s *fakeMonitoringService) GetRecentAuditLogs(ctx context.Context, filter domainauth.AuditLogFilter, pagination common.Pagination) ([]domainauth.AuditLog, error) {
	return nil, nil
}

type fakeHTTPTokenService struct {
	claims *domainauth.AccessClaims
}

func (s *fakeHTTPTokenService) GenerateAccessToken(ctx context.Context, claims domainauth.AccessClaims) (string, time.Time, error) {
	return "", time.Time{}, nil
}

func (s *fakeHTTPTokenService) ValidateAccessToken(ctx context.Context, token string) (*domainauth.AccessClaims, error) {
	return s.claims, nil
}

func (s *fakeHTTPTokenService) GenerateRefreshToken() (plain string, hash string, err error) {
	return "", "", nil
}

func (s *fakeHTTPTokenService) HashRefreshToken(plain string) string {
	return ""
}

func (s *fakeHTTPTokenService) BlacklistAccessToken(ctx context.Context, tokenID string, ttl time.Duration) error {
	return nil
}

func (s *fakeHTTPTokenService) IsAccessTokenBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	return false, nil
}
