package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/remihneppo/be-go-template/internal/config"
	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/monitoring"
	"github.com/remihneppo/be-go-template/internal/middleware"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
	"github.com/remihneppo/be-go-template/internal/platform/metrics"
	"github.com/remihneppo/be-go-template/internal/platform/ratelimit"
)

type ReadinessChecker interface {
	Check(ctx context.Context) (monitoring.DependencyStatus, bool)
}

type RouterDependencies struct {
	AuthService    domainauth.Service
	TokenService   domainauth.TokenService
	Monitoring     monitoring.Service
	HTTPMetrics    *metrics.HTTPMetrics
	RateLimiter    ratelimit.Limiter
	MetricsHandler gin.HandlerFunc
	Readiness      ReadinessChecker
}

func NewRouter(cfg config.Config, log logger.Logger) *gin.Engine {
	return NewRouterWithDependencies(cfg, log, RouterDependencies{})
}

func NewRouterWithDependencies(cfg config.Config, log logger.Logger, deps RouterDependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	httpMetrics := deps.HTTPMetrics
	if cfg.Metrics.Enabled && httpMetrics == nil {
		var err error
		httpMetrics, err = metrics.NewHTTPMetrics(nil, "")
		if err != nil {
			log.Warn("init HTTP metrics failed", logger.Any("error", err))
		}
	}

	router.Use(middleware.Recovery(log))
	router.Use(middleware.RequestID(log))
	router.Use(middleware.CORS(cfg.HTTP.CORSAllowOrigins))
	router.Use(middleware.BodyLimit(cfg.HTTP.BodyLimitBytes))
	router.Use(middleware.Timeout(cfg.HTTP.RouteTimeout))
	if cfg.Metrics.Enabled {
		router.Use(middleware.Metrics(httpMetrics, cfg.Metrics.Path))
	}
	router.Use(middleware.Logging(log))
	router.Use(middleware.ErrorHandler(log))

	router.GET("/healthz", func(c *gin.Context) {
		OK(c, gin.H{"status": "ok", "time": time.Now().UTC()})
	})
	router.GET("/readyz", readyz(deps.Readiness))
	if cfg.Metrics.Enabled {
		router.GET(cfg.Metrics.Path, metricsEndpoint(deps.MetricsHandler))
	}

	v1 := router.Group("/v1")
	if deps.AuthService != nil {
		NewAuthHandler(deps.AuthService, WithAuthRouteMiddleware(authRouteMiddleware(cfg, deps.RateLimiter, deps.TokenService))).RegisterRoutes(v1)
	}
	if deps.Monitoring != nil {
		admin := v1.Group("/admin", middleware.Authenticate(deps.TokenService), middleware.AdminGuard())
		NewMonitoringHandler(deps.Monitoring).RegisterRoutes(admin)
	}

	return router
}

func readyz(checker ReadinessChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if checker == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"data": gin.H{
					"status": monitoring.Unhealthy,
					"dependencies": monitoring.DependencyStatus{
						MongoDB: monitoring.DependencyCheck{Status: monitoring.Unhealthy, Error: "readiness checker not configured", CheckedAt: time.Now().UTC()},
						Redis:   monitoring.DependencyCheck{Status: monitoring.Unhealthy, Error: "readiness checker not configured", CheckedAt: time.Now().UTC()},
					},
				},
			})
			return
		}
		dependencies, ready := checker.Check(c.Request.Context())
		status := monitoring.Healthy
		httpStatus := http.StatusOK
		if !ready {
			status = monitoring.Unhealthy
			httpStatus = http.StatusServiceUnavailable
		} else if dependencies.MongoDB.Status == monitoring.Degraded || dependencies.Redis.Status == monitoring.Degraded || dependencies.Redis.Status == monitoring.Unhealthy {
			status = monitoring.Degraded
		}
		c.JSON(httpStatus, gin.H{
			"success": ready,
			"data": gin.H{
				"status":       status,
				"dependencies": dependencies,
			},
		})
	}
}

func metricsEndpoint(handler gin.HandlerFunc) gin.HandlerFunc {
	if handler != nil {
		return handler
	}
	return gin.WrapH(promhttp.Handler())
}

func authRouteMiddleware(cfg config.Config, limiter ratelimit.Limiter, tokens domainauth.TokenService) AuthRouteMiddleware {
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
		Register:  []gin.HandlerFunc{middleware.RateLimit(limiter, policy(cfg.RateLimit.RegisterPerMinute, ipRateLimitKey("auth:register")))},
		Login:     []gin.HandlerFunc{middleware.RateLimit(limiter, policy(cfg.RateLimit.LoginPerMinute, jsonFieldRateLimitKey("auth:login", "email")))},
		Refresh:   []gin.HandlerFunc{middleware.RateLimit(limiter, policy(cfg.RateLimit.RefreshPerMinute, ipRateLimitKey("auth:refresh")))},
		Protected: []gin.HandlerFunc{middleware.Authenticate(tokens)},
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
