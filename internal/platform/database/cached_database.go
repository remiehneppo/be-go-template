package database

import (
	"context"
	"errors"
	"time"

	"github.com/remihneppo/be-go-template/internal/platform/cache"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
	platformmetrics "github.com/remihneppo/be-go-template/internal/platform/metrics"
)

const defaultWriteLockTTL = 5 * time.Second

type CachedDatabase struct {
	base    Database
	cache   cache.Cache
	log     logger.Logger
	metrics *platformmetrics.DatabaseMetrics
}

func NewCached(base Database, cacheStore cache.Cache, log logger.Logger, metrics *platformmetrics.DatabaseMetrics) *CachedDatabase {
	if log == nil {
		log = logger.NewNoop()
	}
	return &CachedDatabase{base: base, cache: cacheStore, log: log, metrics: metrics}
}

func (d *CachedDatabase) FindOne(ctx context.Context, collection string, filter any, dest any, opts ReadOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if opts.CacheKey == "" || d.cache == nil {
		d.recordCacheEvent("find_one", "disabled")
		return d.callAndLogDependency("CachedDatabase.FindOne", d.base.FindOne(ctx, collection, filter, dest, opts), logger.String("collection", collection))
	}
	return d.readThrough(ctx, opts, dest, func() error {
		return d.callAndLogDependency("CachedDatabase.FindOne", d.base.FindOne(ctx, collection, filter, dest, opts), logger.String("collection", collection), logger.String("cache_key", opts.CacheKey))
	})
}

func (d *CachedDatabase) FindMany(ctx context.Context, collection string, filter any, dest any, opts ReadOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if opts.CacheKey == "" || d.cache == nil {
		d.recordCacheEvent("find_many", "disabled")
		return d.callAndLogDependency("CachedDatabase.FindMany", d.base.FindMany(ctx, collection, filter, dest, opts), logger.String("collection", collection))
	}
	if _, ok := filter.(CacheableFilter); !ok {
		d.log.Warn("FindMany cache skipped because filter is not CacheableFilter", logger.String("cache_key", opts.CacheKey))
		d.recordCacheEvent("find_many", "skipped")
		return d.callAndLogDependency("CachedDatabase.FindMany", d.base.FindMany(ctx, collection, filter, dest, opts), logger.String("collection", collection), logger.String("cache_key", opts.CacheKey))
	}
	return d.readThrough(ctx, opts, dest, func() error {
		return d.callAndLogDependency("CachedDatabase.FindMany", d.base.FindMany(ctx, collection, filter, dest, opts), logger.String("collection", collection), logger.String("cache_key", opts.CacheKey))
	})
}

func (d *CachedDatabase) InsertOne(ctx context.Context, collection string, document any, opts WriteOptions) error {
	return d.write(ctx, opts, func() error {
		return d.callAndLogDependency("CachedDatabase.InsertOne", d.base.InsertOne(ctx, collection, document, opts), logger.String("collection", collection))
	})
}

func (d *CachedDatabase) UpdateOne(ctx context.Context, collection string, filter any, update any, opts WriteOptions) error {
	return d.write(ctx, opts, func() error {
		return d.callAndLogDependency("CachedDatabase.UpdateOne", d.base.UpdateOne(ctx, collection, filter, update, opts), logger.String("collection", collection), logger.String("lock_key", opts.LockKey))
	})
}

func (d *CachedDatabase) UpdateMany(ctx context.Context, collection string, filter any, update any, opts WriteOptions) error {
	return d.write(ctx, opts, func() error {
		return d.callAndLogDependency("CachedDatabase.UpdateMany", d.base.UpdateMany(ctx, collection, filter, update, opts), logger.String("collection", collection), logger.String("lock_key", opts.LockKey))
	})
}

func (d *CachedDatabase) DeleteOne(ctx context.Context, collection string, filter any, opts WriteOptions) error {
	return d.write(ctx, opts, func() error {
		return d.callAndLogDependency("CachedDatabase.DeleteOne", d.base.DeleteOne(ctx, collection, filter, opts), logger.String("collection", collection), logger.String("lock_key", opts.LockKey))
	})
}

func (d *CachedDatabase) Count(ctx context.Context, collection string, filter any) (int64, error) {
	count, err := d.base.Count(ctx, collection, filter)
	return count, d.callAndLogDependency("CachedDatabase.Count", err, logger.String("collection", collection))
}

func (d *CachedDatabase) Ping(ctx context.Context) error {
	return d.callAndLogDependency("CachedDatabase.Ping", d.base.Ping(ctx))
}

func (d *CachedDatabase) Close(ctx context.Context) error {
	if d.cache != nil {
		if err := d.cache.Close(); err != nil {
			d.log.Warn("cache close failed", logger.Any("error", err))
		}
	}
	return d.base.Close(ctx)
}

func (d *CachedDatabase) readThrough(ctx context.Context, opts ReadOptions, dest any, load func() error) error {
	if err := d.cache.Get(ctx, opts.CacheKey, dest); err == nil {
		d.recordCacheEvent("read", "hit")
		d.log.Debug("cache hit", logger.String("cache_key", opts.CacheKey))
		return nil
	} else if !errors.Is(err, cache.ErrCacheMiss) {
		d.recordCacheEvent("read", "get_failed")
		d.log.Warn("cache get failed, falling back to database", logger.String("cache_key", opts.CacheKey), logger.Any("error", err))
	} else {
		d.recordCacheEvent("read", "miss")
		d.log.Debug("cache miss", logger.String("cache_key", opts.CacheKey))
	}

	loadAndSet := func(ctx context.Context) error {
		if err := d.cache.Get(ctx, opts.CacheKey, dest); err == nil {
			d.recordCacheEvent("read", "hit_after_lock")
			d.log.Debug("cache hit after lock", logger.String("cache_key", opts.CacheKey))
			return nil
		}
		d.recordCacheEvent("read", "miss_after_lock")
		d.log.Debug("cache miss after lock", logger.String("cache_key", opts.CacheKey))
		if err := load(); err != nil {
			return err
		}
		if err := d.cache.Set(ctx, opts.CacheKey, dest, opts.CacheTTL); err != nil {
			d.recordCacheEvent("read", "set_failed")
			d.log.Warn("cache set failed", logger.String("cache_key", opts.CacheKey), logger.Any("error", err))
		} else {
			d.recordCacheEvent("read", "set_ok")
			d.log.Debug("cache populated", logger.String("cache_key", opts.CacheKey))
		}
		return nil
	}

	if opts.LockOnMiss {
		start := time.Now()
		if err := d.cache.WithLock(ctx, opts.CacheKey, opts.CacheTTL, loadAndSet); err != nil {
			d.recordCacheEvent("read", "lock_fallback")
			d.observeCacheLock("read", time.Since(start))
			if errors.Is(err, cache.ErrLockNotAcquired) {
				d.log.Warn("cache read lock not acquired, falling back to database", logger.String("cache_key", opts.CacheKey), logger.Any("error", err))
			} else {
				d.log.Warn("cache read lock failed, falling back to database", logger.String("cache_key", opts.CacheKey), logger.Any("error", err))
			}
			if loadErr := load(); loadErr != nil {
				return loadErr
			}
			if err := d.cache.Set(ctx, opts.CacheKey, dest, opts.CacheTTL); err != nil {
				d.recordCacheEvent("read", "set_failed")
				d.log.Warn("cache set failed", logger.String("cache_key", opts.CacheKey), logger.Any("error", err))
			} else {
				d.recordCacheEvent("read", "set_ok")
				d.log.Debug("cache populated", logger.String("cache_key", opts.CacheKey))
			}
			return nil
		}
		d.observeCacheLock("read", time.Since(start))
		return nil
	}

	if err := load(); err != nil {
		return err
	}
	if err := d.cache.Set(ctx, opts.CacheKey, dest, opts.CacheTTL); err != nil {
		d.recordCacheEvent("read", "set_failed")
		d.log.Warn("cache set failed", logger.String("cache_key", opts.CacheKey), logger.Any("error", err))
	} else {
		d.recordCacheEvent("read", "set_ok")
		d.log.Debug("cache populated", logger.String("cache_key", opts.CacheKey))
	}
	return nil
}

func (d *CachedDatabase) write(ctx context.Context, opts WriteOptions, write func() error) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	run := func(ctx context.Context) error {
		if err := write(); err != nil {
			return err
		}
		d.invalidate(ctx, opts.InvalidateKeys)
		if len(opts.InvalidateKeys) > 0 {
			d.recordCacheEvent("write", "invalidated")
			d.log.Debug("cache invalidated", logger.Int("key_count", len(opts.InvalidateKeys)))
		}
		return nil
	}
	if opts.LockKey == "" || d.cache == nil {
		d.recordCacheEvent("write", "disabled")
		return run(ctx)
	}
	start := time.Now()
	if err := d.cache.WithLock(ctx, opts.LockKey, defaultWriteLockTTL, run); err != nil {
		d.observeCacheLock("write", time.Since(start))
		d.recordCacheEvent("write", "lock_fallback")
		if opts.StrictLock {
			return dependencyError("CachedDatabase.write", err)
		}
		d.log.Warn("cache write lock failed, continuing without strict lock", logger.String("lock_key", opts.LockKey), logger.Any("error", err))
		return run(ctx)
	}
	d.observeCacheLock("write", time.Since(start))
	d.recordCacheEvent("write", "lock_acquired")
	d.log.Debug("cache write lock acquired", logger.String("lock_key", opts.LockKey))
	return nil
}

func (d *CachedDatabase) invalidate(ctx context.Context, keys []string) {
	if len(keys) == 0 || d.cache == nil {
		return
	}
	if err := d.cache.Delete(ctx, keys...); err != nil {
		d.recordCacheEvent("write", "invalidation_failed")
		d.log.Warn("cache invalidation failed", logger.Any("error", err))
	}
}

func (d *CachedDatabase) callAndLogDependency(op string, err error, fields ...logger.Field) error {
	if err := dependencyError(op, err); err != nil {
		d.logDependencyError(op, err, fields...)
		return err
	}
	return nil
}

func (d *CachedDatabase) recordCacheEvent(operation, result string) {
	if d.metrics != nil {
		d.metrics.RecordCacheEvent(operation, result)
	}
}

func (d *CachedDatabase) observeCacheLock(operation string, duration time.Duration) {
	if d.metrics != nil {
		d.metrics.ObserveCacheLock(operation, duration)
	}
}

func (d *CachedDatabase) logDependencyError(op string, err error, fields ...logger.Field) {
	if err == nil {
		return
	}
	appErr := apperrors.FromError(err)
	if appErr == nil || appErr.Code != apperrors.CodeDependency {
		return
	}
	fields = append(fields,
		logger.String("op", op),
		logger.String("error_code", string(appErr.Code)),
		logger.Any("error", err),
	)
	d.log.Warn("database dependency error", fields...)
	d.recordDependencyError(op)
}

func (d *CachedDatabase) recordDependencyError(operation string) {
	if d.metrics != nil {
		d.metrics.RecordDependencyError(operation)
	}
}
