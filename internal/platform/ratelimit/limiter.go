package ratelimit

import (
	"context"
	"fmt"
	"time"
)

type Decision struct {
	Allowed    bool
	Limit      int64
	Remaining  int64
	ResetAfter time.Duration
}

type Limiter interface {
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (Decision, error)
}

type Counter interface {
	Increment(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

type RedisLimiter struct {
	counter Counter
}

func NewRedisLimiter(counter Counter) *RedisLimiter {
	return &RedisLimiter{counter: counter}
}

func (l *RedisLimiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (Decision, error) {
	if key == "" {
		return Decision{}, fmt.Errorf("rate limit key must not be empty")
	}
	if limit <= 0 {
		return Decision{Allowed: true, Limit: limit, Remaining: 0, ResetAfter: window}, nil
	}
	if window <= 0 {
		return Decision{}, fmt.Errorf("rate limit window must be positive")
	}
	count, err := l.counter.Increment(ctx, key, window)
	if err != nil {
		return Decision{}, err
	}
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}
	return Decision{
		Allowed:    count <= limit,
		Limit:      limit,
		Remaining:  remaining,
		ResetAfter: window,
	}, nil
}
