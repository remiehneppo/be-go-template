package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/config"
	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

func TestStableETagIsStableForSamePayload(t *testing.T) {
	payload := []map[string]string{{"session_id": "s1", "device_id": "d1"}}

	first := StableETag(payload)
	second := StableETag(payload)

	if first == "" || first != second {
		t.Fatalf("etag = %q, %q", first, second)
	}
}

func TestIfNoneMatchSupportsMultipleValues(t *testing.T) {
	if !ifNoneMatch(`"other", "abc"`, `"abc"`) {
		t.Fatal("ifNoneMatch() = false")
	}
}

func TestRouterDisablesETagHeadersWhenConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Config{}
	cfg.App.Name = "test"
	cfg.HTTP.ETagEnabled = false
	cfg.HTTP.CORSAllowOrigins = []string{"http://localhost:3000"}

	authService := &fakeAuthService{}
	router := NewRouterWithDependencies(cfg, logger.NewNoop(), RouterDependencies{
		AuthService: authService,
		UserService: &fakeUserService{user: &user.User{ID: "u1", Email: "user@example.com", Name: "User", Roles: []user.Role{user.RoleUser}}},
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

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("ETag") != "" {
		t.Fatalf("ETag header = %q", rec.Header().Get("ETag"))
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/auth/devices", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("devices status = %d body = %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("ETag") != "" {
		t.Fatalf("devices ETag header = %q", rec.Header().Get("ETag"))
	}
	if authService.listDevicesUserID != "u1" {
		t.Fatalf("ListDevices user id = %q", authService.listDevicesUserID)
	}
}
