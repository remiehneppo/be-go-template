package database

import (
	"fmt"
	"time"
)

type ReadOptions struct {
	CacheKey   string
	CacheTTL   time.Duration
	LockOnMiss bool
}

type WriteOptions struct {
	LockKey        string
	InvalidateKeys []string
	StrictLock     bool
}

type CacheableFilter interface {
	CacheKeyParts() []string
}

func (o ReadOptions) Validate() error {
	if o.CacheKey == "" && (o.CacheTTL > 0 || o.LockOnMiss) {
		return fmt.Errorf("read cache options require CacheKey")
	}
	if o.CacheKey != "" && o.CacheTTL <= 0 {
		return fmt.Errorf("CacheTTL must be positive when CacheKey is set")
	}
	return nil
}

func (o WriteOptions) Validate() error {
	if o.StrictLock && o.LockKey == "" {
		return fmt.Errorf("StrictLock requires LockKey")
	}
	return nil
}
