package database

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/remihneppo/be-go-template/internal/platform/cache"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

type testDoc struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cacheableFilter struct {
	ID string
}

func (f cacheableFilter) CacheKeyParts() []string {
	return []string{"id", f.ID}
}

func TestCachedDatabaseFindOneCacheHitSkipsBase(t *testing.T) {
	base := &fakeDatabase{findOneValue: testDoc{ID: "base"}}
	cacheStore := newFakeCache()
	cacheStore.values["doc:1"] = testDoc{ID: "cached", Name: "Cached"}
	db := NewCached(base, cacheStore, logger.NewNoop())

	var got testDoc
	err := db.FindOne(context.Background(), "docs", map[string]string{"id": "1"}, &got, ReadOptions{
		CacheKey: "doc:1",
		CacheTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("FindOne() error = %v", err)
	}
	if got.ID != "cached" {
		t.Fatalf("got.ID = %q", got.ID)
	}
	if base.findOneCalls != 0 {
		t.Fatalf("base find calls = %d", base.findOneCalls)
	}
}

func TestCachedDatabaseFindOneMissLoadsAndCaches(t *testing.T) {
	base := &fakeDatabase{findOneValue: testDoc{ID: "base", Name: "Base"}}
	cacheStore := newFakeCache()
	db := NewCached(base, cacheStore, logger.NewNoop())

	var got testDoc
	err := db.FindOne(context.Background(), "docs", map[string]string{"id": "1"}, &got, ReadOptions{
		CacheKey:   "doc:1",
		CacheTTL:   time.Minute,
		LockOnMiss: true,
	})
	if err != nil {
		t.Fatalf("FindOne() error = %v", err)
	}
	if got.ID != "base" {
		t.Fatalf("got.ID = %q", got.ID)
	}
	if base.findOneCalls != 1 {
		t.Fatalf("base find calls = %d", base.findOneCalls)
	}
	if cacheStore.lockCalls != 1 {
		t.Fatalf("lock calls = %d", cacheStore.lockCalls)
	}
	if _, ok := cacheStore.values["doc:1"]; !ok {
		t.Fatal("cache was not populated")
	}
}

func TestCachedDatabaseFindManyRequiresCacheableFilter(t *testing.T) {
	base := &fakeDatabase{findManyValue: []testDoc{{ID: "1"}}}
	cacheStore := newFakeCache()
	db := NewCached(base, cacheStore, logger.NewNoop())

	var got []testDoc
	err := db.FindMany(context.Background(), "docs", map[string]string{"name": "x"}, &got, ReadOptions{
		CacheKey: "docs:list",
		CacheTTL: time.Minute,
		Limit:    25,
		Offset:   10,
		Sort:     map[string]int{"created_at": -1},
	})
	if err != nil {
		t.Fatalf("FindMany() error = %v", err)
	}
	if base.findManyCalls != 1 {
		t.Fatalf("base find many calls = %d", base.findManyCalls)
	}
	if cacheStore.setCalls != 0 {
		t.Fatalf("cache set calls = %d", cacheStore.setCalls)
	}
	if base.lastFindManyOpts.Limit != 25 || base.lastFindManyOpts.Offset != 10 || base.lastFindManyOpts.Sort == nil {
		t.Fatalf("preserved opts = %+v", base.lastFindManyOpts)
	}
}

func TestCachedDatabaseFindManyCachesCacheableFilter(t *testing.T) {
	base := &fakeDatabase{findManyValue: []testDoc{{ID: "1"}}}
	cacheStore := newFakeCache()
	db := NewCached(base, cacheStore, logger.NewNoop())

	var got []testDoc
	err := db.FindMany(context.Background(), "docs", cacheableFilter{ID: "1"}, &got, ReadOptions{
		CacheKey: "docs:id:1",
		CacheTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("FindMany() error = %v", err)
	}
	if cacheStore.setCalls != 1 {
		t.Fatalf("cache set calls = %d", cacheStore.setCalls)
	}
}

func TestCachedDatabaseWriteInvalidatesKeys(t *testing.T) {
	base := &fakeDatabase{}
	cacheStore := newFakeCache()
	db := NewCached(base, cacheStore, logger.NewNoop())

	err := db.UpdateOne(context.Background(), "docs", map[string]string{"id": "1"}, map[string]string{"$set": "x"}, WriteOptions{
		InvalidateKeys: []string{"doc:1", "docs:list"},
	})
	if err != nil {
		t.Fatalf("UpdateOne() error = %v", err)
	}
	if base.updateCalls != 1 {
		t.Fatalf("update calls = %d", base.updateCalls)
	}
	if len(cacheStore.deleted) != 2 {
		t.Fatalf("deleted keys = %v", cacheStore.deleted)
	}
}

func TestCachedDatabaseStrictWriteLockFailsBeforeWrite(t *testing.T) {
	base := &fakeDatabase{}
	cacheStore := newFakeCache()
	cacheStore.lockErr = cache.ErrLockNotAcquired
	db := NewCached(base, cacheStore, logger.NewNoop())

	err := db.UpdateOne(context.Background(), "docs", nil, nil, WriteOptions{
		LockKey:    "doc:1",
		StrictLock: true,
	})
	if !errors.Is(err, cache.ErrLockNotAcquired) {
		t.Fatalf("UpdateOne() error = %v", err)
	}
	if base.updateCalls != 0 {
		t.Fatalf("update calls = %d", base.updateCalls)
	}
}

func TestCachedDatabaseNonStrictWriteLockFallsBack(t *testing.T) {
	base := &fakeDatabase{}
	cacheStore := newFakeCache()
	cacheStore.lockErr = cache.ErrLockNotAcquired
	db := NewCached(base, cacheStore, logger.NewNoop())

	err := db.UpdateOne(context.Background(), "docs", nil, nil, WriteOptions{LockKey: "doc:1"})
	if err != nil {
		t.Fatalf("UpdateOne() error = %v", err)
	}
	if base.updateCalls != 1 {
		t.Fatalf("update calls = %d", base.updateCalls)
	}
}

type fakeDatabase struct {
	findOneValue  any
	findManyValue any

	findOneCalls     int
	findManyCalls    int
	insertCalls      int
	updateCalls      int
	updateManyCalls  int
	deleteCalls      int
	lastFindManyOpts ReadOptions
}

func (d *fakeDatabase) FindOne(ctx context.Context, collection string, filter any, dest any, opts ReadOptions) error {
	d.findOneCalls++
	return copyValue(dest, d.findOneValue)
}

func (d *fakeDatabase) FindMany(ctx context.Context, collection string, filter any, dest any, opts ReadOptions) error {
	d.findManyCalls++
	d.lastFindManyOpts = opts
	return copyValue(dest, d.findManyValue)
}

func (d *fakeDatabase) InsertOne(ctx context.Context, collection string, document any, opts WriteOptions) error {
	d.insertCalls++
	return nil
}

func (d *fakeDatabase) UpdateOne(ctx context.Context, collection string, filter any, update any, opts WriteOptions) error {
	d.updateCalls++
	return nil
}

func (d *fakeDatabase) UpdateMany(ctx context.Context, collection string, filter any, update any, opts WriteOptions) error {
	d.updateManyCalls++
	return nil
}

func (d *fakeDatabase) DeleteOne(ctx context.Context, collection string, filter any, opts WriteOptions) error {
	d.deleteCalls++
	return nil
}

func (d *fakeDatabase) Count(ctx context.Context, collection string, filter any) (int64, error) {
	return 0, nil
}

func (d *fakeDatabase) Ping(ctx context.Context) error {
	return nil
}

func (d *fakeDatabase) Close(ctx context.Context) error {
	return nil
}

type fakeCache struct {
	values    map[string]any
	deleted   []string
	lockErr   error
	setCalls  int
	lockCalls int
}

func newFakeCache() *fakeCache {
	return &fakeCache{values: map[string]any{}}
}

func (c *fakeCache) Get(ctx context.Context, key string, dest any) error {
	value, ok := c.values[key]
	if !ok {
		return cache.ErrCacheMiss
	}
	return copyValue(dest, value)
}

func (c *fakeCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	c.setCalls++
	c.values[key] = value
	return nil
}

func (c *fakeCache) Delete(ctx context.Context, keys ...string) error {
	c.deleted = append(c.deleted, keys...)
	for _, key := range keys {
		delete(c.values, key)
	}
	return nil
}

func (c *fakeCache) Exists(ctx context.Context, key string) (bool, error) {
	_, ok := c.values[key]
	return ok, nil
}

func (c *fakeCache) Increment(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	return 1, nil
}

func (c *fakeCache) WithLock(ctx context.Context, key string, ttl time.Duration, fn func(ctx context.Context) error) error {
	c.lockCalls++
	if c.lockErr != nil {
		return c.lockErr
	}
	return fn(ctx)
}

func (c *fakeCache) Ping(ctx context.Context) error {
	return nil
}

func (c *fakeCache) Close() error {
	return nil
}

func copyValue(dest any, src any) error {
	payload, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, dest)
}
