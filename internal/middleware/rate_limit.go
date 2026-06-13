package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
	"github.com/remihneppo/be-go-template/internal/platform/ratelimit"
)

const (
	RateLimitFallbackAllow = "allow"
	RateLimitFallbackBlock = "block"
)

// RateLimitPolicy configures rate limiting per endpoint.
type RateLimitPolicy struct {
	Enabled  bool
	Limit    int64
	Window   time.Duration
	Fallback string
	KeyFunc  func(*gin.Context) string
}

func RateLimit(limiter ratelimit.Limiter, policy RateLimitPolicy) gin.HandlerFunc {
	if policy.Window <= 0 {
		policy.Window = time.Minute
	}
	if policy.Fallback == "" {
		policy.Fallback = RateLimitFallbackBlock
	}
	return func(c *gin.Context) {
		if !policy.Enabled || limiter == nil || policy.Limit <= 0 {
			c.Next()
			return
		}
		key := ""
		if policy.KeyFunc != nil {
			key = policy.KeyFunc(c)
		}
		if key == "" {
			key = "route:" + c.FullPath() + ":ip:" + c.ClientIP()
		}
		decision, err := limiter.Allow(c.Request.Context(), key, policy.Limit, policy.Window)
		if err != nil {
			logger.FromContext(c.Request.Context()).Warn("rate limiter failed", logger.String("key", key), logger.Any("error", err))
			if policy.Fallback == RateLimitFallbackAllow {
				c.Next()
				return
			}
			writeError(c, apperrors.New(apperrors.CodeRateLimited, "Too many requests", http.StatusTooManyRequests))
			c.Abort()
			return
		}
		c.Header("X-RateLimit-Limit", formatInt(decision.Limit))
		c.Header("X-RateLimit-Remaining", formatInt(decision.Remaining))
		if !decision.Allowed {
			writeError(c, apperrors.New(apperrors.CodeRateLimited, "Too many requests", http.StatusTooManyRequests))
			c.Abort()
			return
		}
		c.Next()
	}
}

func formatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
