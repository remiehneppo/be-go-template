package http

import (
	"context"
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
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("body = %s", rec.Body.String())
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
	if !strings.Contains(body, "testapp_http_requests_total") || !strings.Contains(body, `route="/healthz"`) {
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

func testConfig() config.Config {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	return cfg
}

type fakeAuthService struct {
	result            *domainauth.AuthResult
	listDevicesUserID string
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
	return nil, nil
}

type fakeRouteLimiter struct {
	decision ratelimit.Decision
}

func (l *fakeRouteLimiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (ratelimit.Decision, error) {
	return l.decision, nil
}

type fakeReadiness struct {
	status monitoring.DependencyStatus
	ready  bool
}

func (r *fakeReadiness) Check(ctx context.Context) (monitoring.DependencyStatus, bool) {
	return r.status, r.ready
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
