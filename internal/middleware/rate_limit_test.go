package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/platform/ratelimit"
)

func TestRateLimitRejectsOverLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimit(&fakeLimiter{decision: ratelimit.Decision{Allowed: false, Limit: 1}}, RateLimitPolicy{
		Enabled: true,
		Limit:   1,
		Window:  time.Minute,
		KeyFunc: func(c *gin.Context) string { return "k" },
	}))
	router.GET("/x", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestRateLimitFallbackAllowContinuesWhenLimiterFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimit(&fakeLimiter{err: errors.New("redis down")}, RateLimitPolicy{
		Enabled:  true,
		Limit:    1,
		Window:   time.Minute,
		Fallback: RateLimitFallbackAllow,
		KeyFunc:  func(c *gin.Context) string { return "k" },
	}))
	router.GET("/x", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestRateLimitFallbackBlockRejectsWhenLimiterFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimit(&fakeLimiter{err: errors.New("redis down")}, RateLimitPolicy{
		Enabled:  true,
		Limit:    1,
		Window:   time.Minute,
		Fallback: RateLimitFallbackBlock,
		KeyFunc:  func(c *gin.Context) string { return "k" },
	}))
	router.GET("/x", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

type fakeLimiter struct {
	decision ratelimit.Decision
	err      error
}

func (l *fakeLimiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (ratelimit.Decision, error) {
	if l.err != nil {
		return ratelimit.Decision{}, l.err
	}
	return l.decision, nil
}
