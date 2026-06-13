// Package cache provides a Redis-backed cache abstraction with lock and rate-limit
// primitives. It defines the Cache interface and standard error values used by
// the cached database wrapper and rate limiter.
//
// The interface is intentionally minimal so it can be replaced by a no-op or
// in-memory implementation during tests.
package cache

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrCacheMiss is returned when a cache lookup finds no matching entry.
	ErrCacheMiss = errors.New("cache miss")
	// ErrLockNotAcquired is returned when a distributed lock could not be obtained.
	ErrLockNotAcquired = errors.New("cache lock not acquired")
)

// Cache abstracts a key-value store backed by Redis. Implementations must be
// safe for concurrent use by multiple goroutines.
type Cache interface {
	// Get retrieves the value stored at key and decodes it into dest.
	// Returns ErrCacheMiss when the key does not exist.
	Get(ctx context.Context, key string, dest any) error
	// Set stores value at key with the given TTL. Value is marshalled as JSON.
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	// Delete removes one or more keys from the cache.
	Delete(ctx context.Context, keys ...string) error
	// Exists reports whether key exists in the cache.
	Exists(ctx context.Context, key string) (bool, error)
	// Increment atomically increments the integer value at key, setting TTL
	// on first creation. It returns the new counter value.
	Increment(ctx context.Context, key string, ttl time.Duration) (int64, error)
	// WithLock acquires a distributed lock for key, runs fn, then releases the
	// lock. Returns ErrLockNotAcquired when the lock is held by another owner.
	WithLock(ctx context.Context, key string, ttl time.Duration, fn func(ctx context.Context) error) error
	// Ping checks connectivity to the cache backend.
	Ping(ctx context.Context) error
	// Close releases any underlying resources.
	Close() error
}
