package database

import (
	"context"
	"errors"
	"time"

	"github.com/remihneppo/be-go-template/internal/platform/cache"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

const defaultWriteLockTTL = 5 * time.Second

type CachedDatabase struct {
	base  Database
	cache cache.Cache
	log   logger.Logger
}

func NewCached(base Database, cacheStore cache.Cache, log logger.Logger) *CachedDatabase {
	if log == nil {
		log = logger.NewNoop()
	}
	return &CachedDatabase{base: base, cache: cacheStore, log: log}
}

func (d *CachedDatabase) FindOne(ctx context.Context, collection string, filter any, dest any, opts ReadOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if opts.CacheKey == "" || d.cache == nil {
		return d.base.FindOne(ctx, collection, filter, dest, opts)
	}
	return d.readThrough(ctx, opts, dest, func() error {
		return d.base.FindOne(ctx, collection, filter, dest, opts)
	})
}

func (d *CachedDatabase) FindMany(ctx context.Context, collection string, filter any, dest any, opts ReadOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if opts.CacheKey == "" || d.cache == nil {
		return d.base.FindMany(ctx, collection, filter, dest, opts)
	}
	if _, ok := filter.(CacheableFilter); !ok {
		d.log.Warn("FindMany cache skipped because filter is not CacheableFilter", logger.String("cache_key", opts.CacheKey))
		return d.base.FindMany(ctx, collection, filter, dest, ReadOptions{})
	}
	return d.readThrough(ctx, opts, dest, func() error {
		return d.base.FindMany(ctx, collection, filter, dest, opts)
	})
}

func (d *CachedDatabase) InsertOne(ctx context.Context, collection string, document any, opts WriteOptions) error {
	return d.write(ctx, opts, func() error {
		return d.base.InsertOne(ctx, collection, document, opts)
	})
}

func (d *CachedDatabase) UpdateOne(ctx context.Context, collection string, filter any, update any, opts WriteOptions) error {
	return d.write(ctx, opts, func() error {
		return d.base.UpdateOne(ctx, collection, filter, update, opts)
	})
}

func (d *CachedDatabase) UpdateMany(ctx context.Context, collection string, filter any, update any, opts WriteOptions) error {
	return d.write(ctx, opts, func() error {
		return d.base.UpdateMany(ctx, collection, filter, update, opts)
	})
}

func (d *CachedDatabase) DeleteOne(ctx context.Context, collection string, filter any, opts WriteOptions) error {
	return d.write(ctx, opts, func() error {
		return d.base.DeleteOne(ctx, collection, filter, opts)
	})
}

func (d *CachedDatabase) Ping(ctx context.Context) error {
	return d.base.Ping(ctx)
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
		return nil
	} else if !errors.Is(err, cache.ErrCacheMiss) {
		d.log.Warn("cache get failed, falling back to database", logger.String("cache_key", opts.CacheKey), logger.Any("error", err))
	}

	loadAndSet := func(ctx context.Context) error {
		if err := d.cache.Get(ctx, opts.CacheKey, dest); err == nil {
			return nil
		}
		if err := load(); err != nil {
			return err
		}
		if err := d.cache.Set(ctx, opts.CacheKey, dest, opts.CacheTTL); err != nil {
			d.log.Warn("cache set failed", logger.String("cache_key", opts.CacheKey), logger.Any("error", err))
		}
		return nil
	}

	if opts.LockOnMiss {
		if err := d.cache.WithLock(ctx, opts.CacheKey, opts.CacheTTL, loadAndSet); err != nil {
			d.log.Warn("cache read lock failed, falling back to database", logger.String("cache_key", opts.CacheKey), logger.Any("error", err))
			if loadErr := load(); loadErr != nil {
				return loadErr
			}
			_ = d.cache.Set(ctx, opts.CacheKey, dest, opts.CacheTTL)
			return nil
		}
		return nil
	}

	if err := load(); err != nil {
		return err
	}
	if err := d.cache.Set(ctx, opts.CacheKey, dest, opts.CacheTTL); err != nil {
		d.log.Warn("cache set failed", logger.String("cache_key", opts.CacheKey), logger.Any("error", err))
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
		return nil
	}
	if opts.LockKey == "" || d.cache == nil {
		return run(ctx)
	}
	if err := d.cache.WithLock(ctx, opts.LockKey, defaultWriteLockTTL, run); err != nil {
		if opts.StrictLock {
			return err
		}
		d.log.Warn("cache write lock failed, continuing without strict lock", logger.String("lock_key", opts.LockKey), logger.Any("error", err))
		return run(ctx)
	}
	return nil
}

func (d *CachedDatabase) invalidate(ctx context.Context, keys []string) {
	if len(keys) == 0 || d.cache == nil {
		return
	}
	if err := d.cache.Delete(ctx, keys...); err != nil {
		d.log.Warn("cache invalidation failed", logger.Any("error", err))
	}
}
