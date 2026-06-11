package http

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/config"
	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/middleware"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
	"github.com/remihneppo/be-go-template/internal/platform/ratelimit"
)

type RouterDependencies struct {
	AuthService domainauth.Service
	RateLimiter ratelimit.Limiter
}

func NewRouter(cfg config.Config, log logger.Logger) *gin.Engine {
	return NewRouterWithDependencies(cfg, log, RouterDependencies{})
}

func NewRouterWithDependencies(cfg config.Config, log logger.Logger, deps RouterDependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	router.Use(middleware.Recovery(log))
	router.Use(middleware.RequestID(log))
	router.Use(middleware.CORS(cfg.HTTP.CORSAllowOrigins))
	router.Use(middleware.BodyLimit(cfg.HTTP.BodyLimitBytes))
	router.Use(middleware.Timeout(cfg.HTTP.RouteTimeout))
	router.Use(middleware.Logging(log))
	router.Use(middleware.ErrorHandler(log))

	router.GET("/healthz", func(c *gin.Context) {
		OK(c, gin.H{"status": "ok", "time": time.Now().UTC()})
	})

	v1 := router.Group("/v1")
	if deps.AuthService != nil {
		NewAuthHandler(deps.AuthService, WithAuthRouteMiddleware(authRateLimitMiddleware(cfg, deps.RateLimiter))).RegisterRoutes(v1)
	}

	return router
}

func authRateLimitMiddleware(cfg config.Config, limiter ratelimit.Limiter) AuthRouteMiddleware {
	policy := func(limit int64, keyFunc func(*gin.Context) string) middleware.RateLimitPolicy {
		return middleware.RateLimitPolicy{
			Enabled:  cfg.RateLimit.AuthEnabled,
			Limit:    limit,
			Window:   time.Minute,
			Fallback: cfg.RateLimit.Fallback,
			KeyFunc:  keyFunc,
		}
	}
	return AuthRouteMiddleware{
		Register: []gin.HandlerFunc{middleware.RateLimit(limiter, policy(cfg.RateLimit.RegisterPerMinute, ipRateLimitKey("auth:register")))},
		Login:    []gin.HandlerFunc{middleware.RateLimit(limiter, policy(cfg.RateLimit.LoginPerMinute, jsonFieldRateLimitKey("auth:login", "email")))},
		Refresh:  []gin.HandlerFunc{middleware.RateLimit(limiter, policy(cfg.RateLimit.RefreshPerMinute, ipRateLimitKey("auth:refresh")))},
	}
}

func ipRateLimitKey(prefix string) func(*gin.Context) string {
	return func(c *gin.Context) string {
		return prefix + ":ip:" + c.ClientIP()
	}
}

func jsonFieldRateLimitKey(prefix string, field string) func(*gin.Context) string {
	return func(c *gin.Context) string {
		value := jsonBodyField(c, field)
		if value == "" {
			return prefix + ":ip:" + c.ClientIP()
		}
		return prefix + ":ip:" + c.ClientIP() + ":" + field + ":" + value
	}
}

func jsonBodyField(c *gin.Context, field string) string {
	if c.Request == nil || c.Request.Body == nil {
		return ""
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return ""
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	value, ok := payload[field].(string)
	if !ok {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(value))
}
