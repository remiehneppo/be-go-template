package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/ctxkeys"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
)

type AuthHandler struct {
	service    domainauth.Service
	middleware AuthRouteMiddleware
}

type AuthRouteMiddleware struct {
	Register  []gin.HandlerFunc
	Login     []gin.HandlerFunc
	Refresh   []gin.HandlerFunc
	Protected []gin.HandlerFunc
}

type AuthHandlerOption func(*AuthHandler)

func NewAuthHandler(service domainauth.Service, opts ...AuthHandlerOption) *AuthHandler {
	handler := &AuthHandler{service: service}
	for _, opt := range opts {
		opt(handler)
	}
	return handler
}

func WithAuthRouteMiddleware(routeMiddleware AuthRouteMiddleware) AuthHandlerOption {
	return func(h *AuthHandler) {
		h.middleware = routeMiddleware
	}
}

func (h *AuthHandler) RegisterRoutes(group *gin.RouterGroup) {
	auth := group.Group("/auth")
	auth.POST("/register", appendHandlers(h.middleware.Register, h.Register)...)
	auth.POST("/login", appendHandlers(h.middleware.Login, h.Login)...)
	auth.POST("/refresh", appendHandlers(h.middleware.Refresh, h.Refresh)...)
	auth.POST("/logout", appendHandlers(h.middleware.Protected, h.Logout)...)
	auth.POST("/logout-all", appendHandlers(h.middleware.Protected, h.LogoutAll)...)
	auth.GET("/devices", appendHandlers(h.middleware.Protected, h.ListDevices)...)
	auth.GET("/login-history", appendHandlers(h.middleware.Protected, h.ListLoginHistory)...)
}

func appendHandlers(middleware []gin.HandlerFunc, final gin.HandlerFunc) []gin.HandlerFunc {
	handlers := make([]gin.HandlerFunc, 0, len(middleware)+1)
	handlers = append(handlers, middleware...)
	handlers = append(handlers, final)
	return handlers
}

func (h *AuthHandler) Register(c *gin.Context) {
	var input domainauth.RegisterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		Error(c, validationError(err))
		return
	}
	result, err := h.service.Register(c.Request.Context(), input)
	if err != nil {
		reportContextError(c, err)
		return
	}
	Created(c, authResultResponseFromDomain(result))
}

func (h *AuthHandler) Login(c *gin.Context) {
	var input domainauth.LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		Error(c, validationError(err))
		return
	}
	result, err := h.service.Login(c.Request.Context(), input, requestMeta(c))
	if err != nil {
		reportContextError(c, err)
		return
	}
	OK(c, authResultResponseFromDomain(result))
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		Error(c, validationError(err))
		return
	}
	result, err := h.service.Refresh(c.Request.Context(), body.RefreshToken, requestMeta(c))
	if err != nil {
		reportContextError(c, err)
		return
	}
	OK(c, authResultResponseFromDomain(result))
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var body struct {
		SessionID string `json:"session_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		Error(c, validationError(err))
		return
	}
	sessionID := body.SessionID
	if sessionID == "" {
		sessionID = contextString(c, ctxkeys.SessionID)
	}
	if err := h.service.Logout(c.Request.Context(), bearerToken(c), sessionID); err != nil {
		reportContextError(c, err)
		return
	}
	OK(c, gin.H{"logged_out": true})
}

func (h *AuthHandler) LogoutAll(c *gin.Context) {
	userID := contextString(c, ctxkeys.UserID)
	if userID == "" {
		Error(c, apperrors.New(apperrors.CodeUnauthorized, "Unauthorized", http.StatusUnauthorized))
		return
	}
	if err := h.service.LogoutAll(c.Request.Context(), userID); err != nil {
		reportContextError(c, err)
		return
	}
	OK(c, gin.H{"logged_out": true})
}

func (h *AuthHandler) ListDevices(c *gin.Context) {
	userID := contextString(c, ctxkeys.UserID)
	if userID == "" {
		Error(c, apperrors.New(apperrors.CodeUnauthorized, "Unauthorized", http.StatusUnauthorized))
		return
	}
	devices, err := h.service.ListDevices(c.Request.Context(), userID)
	if err != nil {
		reportContextError(c, err)
		return
	}
	if WriteETagOrNotModified(c, devices) {
		return
	}
	OK(c, devices)
}

func (h *AuthHandler) ListLoginHistory(c *gin.Context) {
	userID := contextString(c, ctxkeys.UserID)
	if userID == "" {
		Error(c, apperrors.New(apperrors.CodeUnauthorized, "Unauthorized", http.StatusUnauthorized))
		return
	}
	history, err := h.service.ListLoginHistory(c.Request.Context(), userID, queryPagination(c).Normalized(20, 100))
	if err != nil {
		reportContextError(c, err)
		return
	}
	OK(c, history)
}

type authResultResponse struct {
	User                  userResponse `json:"user"`
	SessionID             string       `json:"session_id"`
	AccessToken           string       `json:"access_token"`
	AccessTokenExpiresAt  string       `json:"access_token_expires_at"`
	RefreshToken          string       `json:"refresh_token"`
	RefreshTokenExpiresAt string       `json:"refresh_token_expires_at"`
}

type userResponse struct {
	ID    string      `json:"id"`
	Email string      `json:"email"`
	Name  string      `json:"name"`
	Roles []user.Role `json:"roles"`
}

func authResultResponseFromDomain(result *domainauth.AuthResult) authResultResponse {
	if result == nil {
		return authResultResponse{}
	}
	return authResultResponse{
		User:                  userResponseFromDomain(result.User),
		SessionID:             result.SessionID,
		AccessToken:           result.AccessToken,
		AccessTokenExpiresAt:  result.AccessTokenExpiresAt.Format(timeFormat),
		RefreshToken:          result.RefreshToken,
		RefreshTokenExpiresAt: result.RefreshTokenExpiresAt.Format(timeFormat),
	}
}

func userResponseFromDomain(usr user.User) userResponse {
	return userResponse{
		ID:    usr.ID,
		Email: usr.Email,
		Name:  usr.Name,
		Roles: usr.Roles,
	}
}

const timeFormat = "2006-01-02T15:04:05Z07:00"

func requestMeta(c *gin.Context) domainauth.RequestMeta {
	return domainauth.RequestMeta{
		RequestID:  RequestID(c),
		IP:         c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
		DeviceID:   c.GetHeader("X-Device-ID"),
		DeviceName: c.GetHeader("X-Device-Name"),
	}
}

func bearerToken(c *gin.Context) string {
	value := strings.TrimSpace(c.GetHeader("Authorization"))
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return ""
}

func contextString(c *gin.Context, key ctxkeys.Key) string {
	if value, ok := c.Get(string(key)); ok {
		if text, ok := value.(string); ok {
			return text
		}
	}
	if c.Request != nil {
		if text, ok := c.Request.Context().Value(key).(string); ok {
			return text
		}
	}
	return ""
}

func validationError(err error) *apperrors.AppError {
	return apperrors.Validation("Invalid input", []apperrors.ValidationDetail{
		{Field: "body", Reason: "invalid_json"},
	})
}
