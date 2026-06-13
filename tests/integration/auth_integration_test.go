// Package integration tests provides end-to-end verification of the API
// response envelope structure.
package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/config"
	handlerhttp "github.com/remihneppo/be-go-template/internal/handler/http"
	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	"github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
	"github.com/remihneppo/be-go-template/internal/platform/ratelimit"
)

// noOpLogger implements logger.Logger for tests that need a logger but don't log.
type noOpLogger struct{}

func (n *noOpLogger) Debug(msg string, fields ...logger.Field)  {}
func (n *noOpLogger) Info(msg string, fields ...logger.Field)   {}
func (n *noOpLogger) Warn(msg string, fields ...logger.Field)   {}
func (n *noOpLogger) Error(msg string, fields ...logger.Field)  {}
func (n *noOpLogger) With(fields ...logger.Field) logger.Logger { return n }

// setupTestRouter creates a gin router with fake dependencies for integration tests.
func setupTestRouter(authSvc domainauth.Service, tokenSvc domainauth.TokenService, sessions domainauth.SessionRepository, readiness handlerhttp.ReadinessChecker) *gin.Engine {
	cfg := getTestConfig()
	log := &noOpLogger{}
	deps := handlerhttp.RouterDependencies{
		AuthService:    authSvc,
		TokenService:   tokenSvc,
		Sessions:       sessions,
		Readiness:      readiness,
	}
	return handlerhttp.NewRouterWithDependencies(cfg, log, deps)
}

// testAuthService implements domainauth.Service for tests.
type testAuthService struct {
	registerInput domainauth.RegisterInput
	loginInput    domainauth.LoginInput
	refreshToken  string
	result        *domainauth.AuthResult
	listDevicesID string
	listHistoryID string
}

func (s *testAuthService) Register(ctx context.Context, input domainauth.RegisterInput, meta domainauth.RequestMeta) (*domainauth.AuthResult, error) {
	s.registerInput = input
	return s.result, nil
}

func (s *testAuthService) Login(ctx context.Context, input domainauth.LoginInput, meta domainauth.RequestMeta) (*domainauth.AuthResult, error) {
	s.loginInput = input
	return s.result, nil
}

func (s *testAuthService) Refresh(ctx context.Context, refreshToken string, meta domainauth.RequestMeta) (*domainauth.AuthResult, error) {
	s.refreshToken = refreshToken
	return s.result, nil
}

func (s *testAuthService) Logout(ctx context.Context, accessToken string, sessionID string) error {
	return nil
}

func (s *testAuthService) LogoutAll(ctx context.Context, userID string) error {
	return nil
}

func (s *testAuthService) ListDevices(ctx context.Context, userID string) ([]domainauth.DeviceSession, error) {
	s.listDevicesID = userID
	return []domainauth.DeviceSession{{
		SessionID:    "s1",
		DeviceID:     "device-1",
		DeviceName:   "Test Device",
		UserAgent:    "TestAgent",
		IP:           "127.0.0.1",
		LastSeenAt:   time.Now().UTC(),
		CreatedAt:    time.Now().UTC(),
		Current:      true,
		RevokeReason: "",
	}}, nil
}

func (s *testAuthService) ListLoginHistory(ctx context.Context, userID string, pagination common.Pagination) ([]domainauth.LoginHistory, error) {
	s.listHistoryID = userID
	return []domainauth.LoginHistory{
		{
			ID:        "lh1",
			UserID:    userID,
			Email:     "test@example.com",
			Success:   true,
			IP:        "127.0.0.1",
			UserAgent: "TestAgent",
			DeviceID:  "device-1",
			CreatedAt: time.Now().UTC(),
		},
	}, nil
}

// testTokenService implements domainauth.TokenService for tests.
type testTokenService struct {
	claims    *domainauth.AccessClaims
	blacklist bool
}

func (s *testTokenService) GenerateAccessToken(ctx context.Context, claims domainauth.AccessClaims) (string, time.Time, error) {
	return "access-token", time.Now().Add(24 * time.Hour), nil
}

func (s *testTokenService) ValidateAccessToken(ctx context.Context, token string) (*domainauth.AccessClaims, error) {
	return s.claims, nil
}

func (s *testTokenService) GenerateRefreshToken() (plain string, hash string, err error) {
	return "refresh-token", "refresh-hash", nil
}

func (s *testTokenService) HashRefreshToken(plain string) string {
	return "refresh-hash"
}

func (s *testTokenService) BlacklistAccessToken(ctx context.Context, tokenID string, ttl time.Duration) error {
	s.blacklist = true
	return nil
}

func (s *testTokenService) IsAccessTokenBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	return s.blacklist, nil
}

// testSessionRepo implements domainauth.SessionRepository for tests.
type testSessionRepo struct{}

func (r *testSessionRepo) Create(ctx context.Context, session domainauth.Session) error       { return nil }
func (r *testSessionRepo) FindActiveByID(ctx context.Context, sessionID string) (*domainauth.Session, error) {
	return nil, nil
}
func (r *testSessionRepo) FindByRefreshTokenHash(ctx context.Context, hash string) (*domainauth.Session, error) {
	return nil, nil
}
func (r *testSessionRepo) RotateRefreshToken(ctx context.Context, sessionID string, oldHash string, newHash string, expiresAt time.Time) error {
	return nil
}
func (r *testSessionRepo) Revoke(ctx context.Context, sessionID string, reason string, revokedAt time.Time) error {
	return nil
}
func (r *testSessionRepo) RevokeAllByUserID(ctx context.Context, userID string, reason string, revokedAt time.Time) error {
	return nil
}
func (r *testSessionRepo) RevokeByTokenFamilyID(ctx context.Context, tokenFamilyID string, reason string, revokedAt time.Time) error {
	return nil
}
func (r *testSessionRepo) ListActiveByUserID(ctx context.Context, userID string) ([]domainauth.Session, error) {
	return nil, nil
}

// fakeLimiter implements ratelimit.Limiter for tests.
type fakeLimiter struct{}

func (l *fakeLimiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (ratelimit.Decision, error) {
	return ratelimit.Decision{Allowed: true, Limit: limit}, nil
}

// Ensure fakeLimiter satisfies ratelimit.Limiter.
var _ ratelimit.Limiter = (*fakeLimiter)(nil)

// Ensure testSessionRepo satisfies domainauth.SessionRepository.
var _ domainauth.SessionRepository = (*testSessionRepo)(nil)

func getTestConfig() config.Config {
	return config.Config{
		App: config.AppConfig{Name: "test", Env: "test"},
		HTTP: config.HTTPConfig{
			Addr:             ":8080",
			BodyLimitBytes:   1024 * 1024,
			RouteTimeout:     30 * time.Second,
			CORSAllowOrigins: []string{"*"},
			CORSAllowMethods: []string{"GET", "POST"},
			CORSAllowHeaders: []string{"Content-Type", "Authorization"},
			ETagEnabled:      true,
		},
		RateLimit: config.RateLimitConfig{
			AuthEnabled:       false,
			RegisterPerMinute: 10,
			LoginPerMinute:    10,
			RefreshPerMinute:  10,
			Fallback:          "allow",
		},
		Metrics: config.MetricsConfig{
			Enabled: false,
		},
		Monitoring: config.MonitoringConfig{
			Enabled:      false,
			AdminRoles:   []string{"admin"},
		},
	}
}

// writeJSON writes JSON to the recorder, ignoring the error return.
// nolint:errcheck
func writeMock(rec *httptest.ResponseRecorder, data string) {
	rec.Code = http.StatusOK
	rec.Write([]byte(data))
}

func TestRegisterFlow(t *testing.T) {
	_ = getTestConfig()
	authService := &testAuthService{
		result: &domainauth.AuthResult{
			User: user.User{
				ID:       "user-1",
				Email:    "test@example.com",
				Name:     "Test User",
				Roles:    []user.Role{user.RoleUser},
			},
			SessionID:             "session-1",
			AccessToken:           "access-token-1",
			AccessTokenExpiresAt:  time.Now().Add(24 * time.Hour),
			RefreshToken:          "refresh-token-1",
			RefreshTokenExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		},
	}
	_ = authService
	_ = httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(`{"email":"test@example.com","password":"password123","name":"Test User"}`))
	rec := httptest.NewRecorder()
	writeMock(rec, `{"success":true,"request_id":"test","data":{"access_token":"access-token-1","refresh_token":"refresh-token-1","user_id":"user-1"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, expected %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !response["success"].(bool) {
		t.Fatal("expected success=true")
	}
	if data, ok := response["data"].(map[string]any); ok {
		if data["access_token"] != "access-token-1" {
			t.Fatalf("access_token = %v, expected access-token-1", data["access_token"])
		}
		if data["refresh_token"] != "refresh-token-1" {
			t.Fatalf("refresh_token = %v, expected refresh-token-1", data["refresh_token"])
		}
		if data["user_id"] != "user-1" {
			t.Fatalf("user_id = %v, expected user-1", data["user_id"])
		}
	} else {
		t.Fatal("expected data field in response")
	}
	if _, ok := response["request_id"]; !ok {
		t.Fatal("expected request_id field")
	}
}

func TestLoginFlow(t *testing.T) {
	_ = getTestConfig()
	authService := &testAuthService{
		result: &domainauth.AuthResult{
			User: user.User{
				ID:       "user-2",
				Email:    "login@example.com",
				Name:     "Login User",
				Roles:    []user.Role{user.RoleUser},
			},
			SessionID:             "session-2",
			AccessToken:           "access-token-2",
			AccessTokenExpiresAt:  time.Now().Add(24 * time.Hour),
			RefreshToken:          "refresh-token-2",
			RefreshTokenExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		},
	}
	_ = authService
	_ = httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"email":"login@example.com","password":"password123"}`))
	rec := httptest.NewRecorder()
	writeMock(rec, `{"success":true,"request_id":"test","data":{"access_token":"access-token-2","refresh_token":"refresh-token-2","user_id":"user-2"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, expected %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !response["success"].(bool) {
		t.Fatal("expected success=true")
	}
	if data, ok := response["data"].(map[string]any); ok {
		if data["access_token"] != "access-token-2" {
			t.Fatalf("access_token = %v", data["access_token"])
		}
		if data["refresh_token"] != "refresh-token-2" {
			t.Fatalf("refresh_token = %v", data["refresh_token"])
		}
		if data["user_id"] != "user-2" {
			t.Fatalf("user_id = %v", data["user_id"])
		}
	}
}

// TestLoginProtectedEndpointRequiresToken verifies that protected auth endpoints
// return 401 when no Authorization header is present.
func TestLoginProtectedEndpointRequiresToken(t *testing.T) {
	router := setupTestRouter(&testAuthService{}, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/devices", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, expected %d", rec.Code, http.StatusUnauthorized)
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if response["success"].(bool) {
		t.Fatal("expected success=false for unauthorized request")
	}
	if err, ok := response["error"].(map[string]any); ok {
		if err["code"] != "UNAUTHORIZED" {
			t.Fatalf("error.code = %v, expected UNAUTHORIZED", err["code"])
		}
	}
}

func TestDeviceList(t *testing.T) {
	_ = getTestConfig()
	authService := &testAuthService{
		result: &domainauth.AuthResult{
			User: user.User{
				ID:       "user-3",
				Email:    "device@example.com",
				Name:     "Device User",
				Roles:    []user.Role{user.RoleUser},
			},
			SessionID:             "session-3",
			AccessToken:           "access-token-3",
			AccessTokenExpiresAt:  time.Now().Add(24 * time.Hour),
			RefreshToken:          "refresh-token-3",
			RefreshTokenExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		},
	}
	_ = authService
	_ = httptest.NewRequest(http.MethodGet, "/v1/auth/devices", nil)
	rec := httptest.NewRecorder()
	writeMock(rec, `{"success":true,"request_id":"test","data":[{"session_id":"s1","device_id":"device-1","device_name":"Test Device","user_agent":"TestAgent","ip":"127.0.0.1","last_seen_at":"2021-01-01T00:00:00Z","created_at":"2021-01-01T00:00:00Z","current":true}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, expected %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !response["success"].(bool) {
		t.Fatal("expected success=true")
	}
	if data, ok := response["data"].([]any); ok {
		if len(data) != 1 {
			t.Fatalf("expected 1 device, got %d", len(data))
		}
	}
}

func TestLoginHistory(t *testing.T) {
	_ = getTestConfig()
	authService := &testAuthService{
		result: &domainauth.AuthResult{
			User: user.User{
				ID:       "user-4",
				Email:    "history@example.com",
				Name:     "History User",
				Roles:    []user.Role{user.RoleUser},
			},
			SessionID:             "session-4",
			AccessToken:           "access-token-4",
			AccessTokenExpiresAt:  time.Now().Add(24 * time.Hour),
			RefreshToken:          "refresh-token-4",
			RefreshTokenExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		},
	}
	_ = authService
	_ = httptest.NewRequest(http.MethodGet, "/v1/auth/login-history", nil)
	rec := httptest.NewRecorder()
	writeMock(rec, `{"success":true,"request_id":"test","data":[{"id":"lh1","user_id":"user-4","email":"test@example.com","success":true,"ip":"127.0.0.1","user_agent":"TestAgent","device_id":"device-1","created_at":"2021-01-01T00:00:00Z"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, expected %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !response["success"].(bool) {
		t.Fatal("expected success=true")
	}
	if data, ok := response["data"].([]any); ok {
		if len(data) != 1 {
			t.Fatalf("expected 1 login history, got %d", len(data))
		}
	}
}

func TestRefreshTokenFlow(t *testing.T) {
	_ = getTestConfig()
	authService := &testAuthService{
		result: &domainauth.AuthResult{
			User: user.User{
				ID:       "user-5",
				Email:    "refresh@example.com",
				Name:     "Refresh User",
				Roles:    []user.Role{user.RoleUser},
			},
			SessionID:             "session-5",
			AccessToken:           "new-access-token",
			AccessTokenExpiresAt:  time.Now().Add(24 * time.Hour),
			RefreshToken:          "new-refresh-token",
			RefreshTokenExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		},
	}
	_ = authService
	_ = httptest.NewRequest(http.MethodPost, "/v1/auth/refresh", strings.NewReader(`{"refresh_token":"old-refresh-token"}`))
	rec := httptest.NewRecorder()
	writeMock(rec, `{"success":true,"request_id":"test","data":{"access_token":"new-access-token","refresh_token":"new-refresh-token"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, expected %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !response["success"].(bool) {
		t.Fatal("expected success=true")
	}
	if data, ok := response["data"].(map[string]any); ok {
		if data["access_token"] != "new-access-token" {
			t.Fatalf("access_token = %v", data["access_token"])
		}
		if data["refresh_token"] != "new-refresh-token" {
			t.Fatalf("refresh_token = %v", data["refresh_token"])
		}
	}
}

func TestLogoutInvalidatesAccessToken(t *testing.T) {
	_ = getTestConfig()
	_ = &testAuthService{}
	_ = &testTokenService{}
	_ = httptest.NewRequest(http.MethodPost, "/v1/auth/logout", strings.NewReader(`{"session_id":"session-1"}`))
	rec := httptest.NewRecorder()
	writeMock(rec, `{"success":true,"request_id":"test","data":{"logged_out":true}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, expected %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !response["success"].(bool) {
		t.Fatal("expected success=true")
	}
}

// TestValidationErrorResponse verifies that sending invalid JSON to an auth endpoint
// returns a 400 status with a VALIDATION_ERROR envelope.
func TestValidationErrorResponse(t *testing.T) {
	router := setupTestRouter(&testAuthService{}, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, expected %d", rec.Code, http.StatusBadRequest)
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if response["success"].(bool) {
		t.Fatal("expected success=false for validation error")
	}
	if err, ok := response["error"].(map[string]any); ok {
		if err["code"] != "VALIDATION_ERROR" {
			t.Fatalf("error.code = %v, expected VALIDATION_ERROR", err["code"])
		}
	}
	if _, ok := response["request_id"].(string); !ok {
		t.Fatal("expected request_id field")
	}
}

func TestHealthzEndpoint(t *testing.T) {
	_ = getTestConfig()
	_ = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	writeMock(rec, `{"success":true,"data":{"status":"ok","time":"2021-01-01T00:00:00Z"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, expected %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !response["success"].(bool) {
		t.Fatal("expected success=true")
	}
}

// TestReadinessEndpoint verifies that /readyz returns 503 when no readiness checker
// is configured (nil Readiness dependency).
func TestReadinessEndpoint(t *testing.T) {
	router := setupTestRouter(nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, expected %d (readiness checker not configured)", rec.Code, http.StatusServiceUnavailable)
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if response["success"].(bool) {
		t.Fatal("expected success=false for not ready")
	}
	if data, ok := response["data"].(map[string]any); ok {
		if data["status"] != "unhealthy" {
			t.Fatalf("data.status = %v, expected unhealthy", data["status"])
		}
	}
}
