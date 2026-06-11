package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestRedisLimiterAllowsUntilLimit(t *testing.T) {
	cacheStore := &fakeCache{counts: map[string]int64{}}
	limiter := NewRedisLimiter(cacheStore)

	first, err := limiter.Allow(context.Background(), "login:ip", 2, time.Minute)
	if err != nil {
		t.Fatalf("Allow() first error = %v", err)
	}
	second, err := limiter.Allow(context.Background(), "login:ip", 2, time.Minute)
	if err != nil {
		t.Fatalf("Allow() second error = %v", err)
	}
	third, err := limiter.Allow(context.Background(), "login:ip", 2, time.Minute)
	if err != nil {
		t.Fatalf("Allow() third error = %v", err)
	}
	if !first.Allowed || !second.Allowed || third.Allowed {
		t.Fatalf("decisions = %+v %+v %+v", first, second, third)
	}
}

type fakeCache struct {
	counts map[string]int64
}

func (c *fakeCache) Increment(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	c.counts[key]++
	return c.counts[key], nil
}
