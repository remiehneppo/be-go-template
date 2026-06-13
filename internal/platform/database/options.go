package database

import (
	"fmt"
	"time"
)

// ReadOptions controls cache and lock behaviour for read operations.
// Zero-value ReadOptions is safe and performs a direct MongoDB read.
type ReadOptions struct {
	// CacheKey identifies the cache entry for this read. An empty value
	// disables caching entirely.
	CacheKey string
	// CacheTTL sets the TTL for the cache entry. Must be positive when CacheKey is set.
	CacheTTL time.Duration
	// LockOnMiss requests a Redis lock on cache miss to prevent stampede.
	// When the lock cannot be acquired the read falls back to direct MongoDB.
	LockOnMiss bool
	// Limit caps the number of returned documents.
	Limit int64
	// Offset skips the first N documents.
	Offset int64
	// Sort applies an ordering expression (MongoDB-style document).
	Sort any
}

// Validate checks internal consistency of ReadOptions. It returns an error
// when CacheTTL or LockOnMiss are set without a CacheKey, or when CacheTTL
// is non-positive while CacheKey is set.
func (o ReadOptions) Validate() error {
	if o.CacheKey == "" && (o.CacheTTL > 0 || o.LockOnMiss) {
		return fmt.Errorf("read cache options require CacheKey")
	}
	if o.CacheKey != "" && o.CacheTTL <= 0 {
		return fmt.Errorf("CacheTTL must be positive when CacheKey is set")
	}
	return nil
}

// WriteOptions controls lock and invalidation behaviour for write operations.
// Zero-value WriteOptions is safe and performs an unrestricted write.
type WriteOptions struct {
	// LockKey is the Redis lock key used for write serialization.
	LockKey string
	// InvalidateKeys lists cache keys to invalidate after a successful write.
	InvalidateKeys []string
	// StrictLock causes the write to fail with DEPENDENCY_ERROR when the lock
	// cannot be acquired. When false the write proceeds and invalidation is
	// best-effort.
	StrictLock bool
}

// Validate checks internal consistency of WriteOptions. It returns an error
// when StrictLock is true but LockKey is empty.
func (o WriteOptions) Validate() error {
	if o.StrictLock && o.LockKey == "" {
		return fmt.Errorf("StrictLock requires LockKey")
	}
	return nil
}

// CacheableFilter is implemented by filters that produce deterministic cache
// keys regardless of pagination or sorting parameters.
type CacheableFilter interface {
	CacheKeyParts() []string
}
