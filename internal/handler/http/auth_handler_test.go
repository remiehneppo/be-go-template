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
	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	"github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
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

func testConfig() config.Config {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	return cfg
}

type fakeAuthService struct {
	result *domainauth.AuthResult
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
	return nil, nil
}

func (s *fakeAuthService) ListLoginHistory(ctx context.Context, userID string, pagination common.Pagination) ([]domainauth.LoginHistory, error) {
	return nil, nil
}
