package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/platform/ctxkeys"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

func TestAuthenticatePopulatesContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := &captureLogger{}
	tokens := &fakeTokenService{claims: &domainauth.AccessClaims{
		UserID:    "u1",
		SessionID: "s1",
		TokenID:   "jti1",
		Roles:     []string{"user"},
	}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := logger.WithContext(c.Request.Context(), capture)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.Use(Authenticate(tokens, nil))
	router.GET("/me", func(c *gin.Context) {
		if c.GetString(string(ctxkeys.UserID)) != "u1" {
			t.Fatalf("gin user id = %q", c.GetString(string(ctxkeys.UserID)))
		}
		if got, _ := c.Request.Context().Value(ctxkeys.SessionID).(string); got != "s1" {
			t.Fatalf("context session id = %q", got)
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !capture.hasField("user_id", "u1") || !capture.hasField("session_id", "s1") || !capture.hasField("token_id", "jti1") || !capture.hasField("roles", []string{"user"}) {
		t.Fatalf("logger fields = %+v", capture.fields)
	}
}

func TestAuthenticateRejectsMissingBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Authenticate(&fakeTokenService{}, nil))
	router.GET("/me", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/me", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAuthenticatePropagatesTokenExpiredError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Authenticate(&fakeTokenService{err: apperrors.TokenExpired("expired")}, nil))
	router.GET("/me", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "TOKEN_EXPIRED") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestAuthenticatePropagatesTokenRevokedError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Authenticate(&fakeTokenService{err: apperrors.TokenRevoked("revoked")}, nil))
	router.GET("/me", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "TOKEN_REVOKED") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestAdminGuardAllowsOnlyAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ctxkeys.Roles), []string{"admin"})
		c.Next()
	})
	router.Use(AdminGuard())
	router.GET("/admin", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminGuardAllowsConfiguredRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ctxkeys.Roles), []string{"ops"})
		c.Next()
	})
	router.Use(AdminGuard("ops", "auditor"))
	router.GET("/admin", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminGuardRejectsUserRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ctxkeys.Roles), []string{"user"})
		c.Next()
	})
	router.Use(AdminGuard())
	router.GET("/admin", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

type fakeTokenService struct {
	claims *domainauth.AccessClaims
	err    error
}

func (s *fakeTokenService) GenerateAccessToken(ctx context.Context, claims domainauth.AccessClaims) (string, time.Time, error) {
	return "", time.Time{}, nil
}

func (s *fakeTokenService) ValidateAccessToken(ctx context.Context, token string) (*domainauth.AccessClaims, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.claims, nil
}

func (s *fakeTokenService) GenerateRefreshToken() (plain string, hash string, err error) {
	return "", "", nil
}

func (s *fakeTokenService) HashRefreshToken(plain string) string {
	return ""
}

func (s *fakeTokenService) BlacklistAccessToken(ctx context.Context, tokenID string, ttl time.Duration) error {
	return nil
}

func (s *fakeTokenService) IsAccessTokenBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	return false, nil
}

type captureLogger struct {
	fields []logger.Field
}

func (l *captureLogger) Debug(string, ...logger.Field) {}
func (l *captureLogger) Info(string, ...logger.Field)  {}
func (l *captureLogger) Warn(string, ...logger.Field)  {}
func (l *captureLogger) Error(string, ...logger.Field) {}

func (l *captureLogger) With(fields ...logger.Field) logger.Logger {
	l.fields = append(l.fields, fields...)
	return l
}

func (l *captureLogger) hasField(key string, want any) bool {
	for _, field := range l.fields {
		if field.Key == key && equalFieldValue(field.Value, want) {
			return true
		}
	}
	return false
}

func equalFieldValue(got any, want any) bool {
	switch wantValue := want.(type) {
	case []string:
		gotValue, ok := got.([]string)
		if !ok || len(gotValue) != len(wantValue) {
			return false
		}
		for i := range gotValue {
			if gotValue[i] != wantValue[i] {
				return false
			}
		}
		return true
	default:
		return got == want
	}
}
