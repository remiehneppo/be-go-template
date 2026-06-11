package cache

import (
	"context"
	"errors"
	"time"
)

var (
	ErrCacheMiss       = errors.New("cache miss")
	ErrLockNotAcquired = errors.New("cache lock not acquired")
)

type Cache interface {
	Get(ctx context.Context, key string, dest any) error
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
	WithLock(ctx context.Context, key string, ttl time.Duration, fn func(ctx context.Context) error) error
	Close() error
}
