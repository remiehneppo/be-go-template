package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/ctxkeys"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
)

func Authenticate(tokens domainauth.TokenService, sessions domainauth.SessionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawToken := bearerToken(c.GetHeader("Authorization"))
		if rawToken == "" || tokens == nil {
			writeError(c, apperrors.New(apperrors.CodeUnauthorized, "Unauthorized", http.StatusUnauthorized))
			c.Abort()
			return
		}
		claims, err := tokens.ValidateAccessToken(c.Request.Context(), rawToken)
		if err != nil || claims == nil || claims.UserID == "" {
			writeError(c, apperrors.New(apperrors.CodeUnauthorized, "Unauthorized", http.StatusUnauthorized))
			c.Abort()
			return
		}
		if sessions != nil {
			session, err := sessions.FindActiveByID(c.Request.Context(), claims.SessionID)
			if err != nil {
				if errors.Is(err, database.ErrNotFound) {
					writeError(c, apperrors.New(apperrors.CodeUnauthorized, "Unauthorized", http.StatusUnauthorized))
				} else {
					writeError(c, apperrors.New(apperrors.CodeDependency, "Dependency error", http.StatusServiceUnavailable))
				}
				c.Abort()
				return
			}
			if session == nil {
				writeError(c, apperrors.New(apperrors.CodeUnauthorized, "Unauthorized", http.StatusUnauthorized))
				c.Abort()
				return
			}
		}
		setAuthContext(c, claims)
		c.Next()
	}
}

func AdminGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !hasRole(c, string(user.RoleAdmin)) {
			writeError(c, apperrors.New(apperrors.CodeForbidden, "Forbidden", http.StatusForbidden))
			c.Abort()
			return
		}
		c.Next()
	}
}

func setAuthContext(c *gin.Context, claims *domainauth.AccessClaims) {
	c.Set(string(ctxkeys.UserID), claims.UserID)
	c.Set(string(ctxkeys.SessionID), claims.SessionID)
	c.Set(string(ctxkeys.TokenID), claims.TokenID)
	c.Set(string(ctxkeys.Roles), claims.Roles)

	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, ctxkeys.UserID, claims.UserID)
	ctx = context.WithValue(ctx, ctxkeys.SessionID, claims.SessionID)
	ctx = context.WithValue(ctx, ctxkeys.TokenID, claims.TokenID)
	ctx = context.WithValue(ctx, ctxkeys.Roles, claims.Roles)
	c.Request = c.Request.WithContext(ctx)
}

func hasRole(c *gin.Context, role string) bool {
	for _, candidate := range contextRoles(c) {
		if candidate == role {
			return true
		}
	}
	return false
}

func contextRoles(c *gin.Context) []string {
	if value, ok := c.Get(string(ctxkeys.Roles)); ok {
		if roles, ok := value.([]string); ok {
			return roles
		}
	}
	if c.Request != nil {
		if roles, ok := c.Request.Context().Value(ctxkeys.Roles).([]string); ok {
			return roles
		}
	}
	return nil
}

func bearerToken(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return ""
}
