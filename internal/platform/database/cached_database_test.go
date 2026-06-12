package database

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/remihneppo/be-go-template/internal/platform/cache"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
	platformmetrics "github.com/remihneppo/be-go-template/internal/platform/metrics"
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
	capture := newDBCaptureLogger()
	cacheStore.values["doc:1"] = testDoc{ID: "cached", Name: "Cached"}
	registry := prometheus.NewRegistry()
	metrics, err := platformmetrics.NewDatabaseMetrics(registry, "testapp")
	if err != nil {
		t.Fatalf("NewDatabaseMetrics() error = %v", err)
	}
	db := NewCached(base, cacheStore, capture, metrics, false)

	var got testDoc
	err = db.FindOne(context.Background(), "docs", map[string]string{"id": "1"}, &got, ReadOptions{
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
	if !capture.hasEntry("debug", "cache hit") || !capture.hasField("cache_key", "doc:1") {
		t.Fatalf("logger entries = %+v", capture.entries)
	}
	if err := testutil.GatherAndCompare(registry, strings.NewReader(`
# HELP testapp_database_cache_events_total Total database cache events grouped by operation and result.
# TYPE testapp_database_cache_events_total counter
testapp_database_cache_events_total{operation="read",result="hit"} 1
`), "testapp_database_cache_events_total"); err != nil {
		t.Fatalf("GatherAndCompare() error = %v", err)
	}
}

func TestCachedDatabaseFindOneMissLoadsAndCaches(t *testing.T) {
	base := &fakeDatabase{findOneValue: testDoc{ID: "base", Name: "Base"}}
	cacheStore := newFakeCache()
	capture := newDBCaptureLogger()
	db := NewCached(base, cacheStore, capture, nil, false)

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
	if !capture.hasEntry("debug", "cache miss") || !capture.hasEntry("debug", "cache populated") {
		t.Fatalf("logger entries = %+v", capture.entries)
	}
}

func TestCachedDatabaseFindManyRequiresCacheableFilter(t *testing.T) {
	base := &fakeDatabase{findManyValue: []testDoc{{ID: "1"}}}
	cacheStore := newFakeCache()
	db := NewCached(base, cacheStore, logger.NewNoop(), nil, false)

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

func TestCachedDatabaseFindManyRejectsNonCacheableFilterInStrictMode(t *testing.T) {
	base := &fakeDatabase{findManyValue: []testDoc{{ID: "1"}}}
	cacheStore := newFakeCache()
	db := NewCached(base, cacheStore, logger.NewNoop(), nil, true)

	var got []testDoc
	err := db.FindMany(context.Background(), "docs", map[string]string{"name": "x"}, &got, ReadOptions{
		CacheKey: "docs:list",
		CacheTTL: time.Minute,
	})
	if err == nil {
		t.Fatal("FindMany() error = nil")
	}
	if base.findManyCalls != 0 {
		t.Fatalf("base find many calls = %d", base.findManyCalls)
	}
	if cacheStore.setCalls != 0 {
		t.Fatalf("cache set calls = %d", cacheStore.setCalls)
	}
}

func TestCachedDatabaseFindManyCachesCacheableFilter(t *testing.T) {
	base := &fakeDatabase{findManyValue: []testDoc{{ID: "1"}}}
	cacheStore := newFakeCache()
	db := NewCached(base, cacheStore, logger.NewNoop(), nil, false)

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
	db := NewCached(base, cacheStore, logger.NewNoop(), nil, false)

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
	db := NewCached(base, cacheStore, logger.NewNoop(), nil, false)

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
	db := NewCached(base, cacheStore, logger.NewNoop(), nil, false)

	err := db.UpdateOne(context.Background(), "docs", nil, nil, WriteOptions{LockKey: "doc:1"})
	if err != nil {
		t.Fatalf("UpdateOne() error = %v", err)
	}
	if base.updateCalls != 1 {
		t.Fatalf("update calls = %d", base.updateCalls)
	}
}

func TestCachedDatabaseReadLockNotAcquiredLogsFallback(t *testing.T) {
	base := &fakeDatabase{findOneValue: testDoc{ID: "base"}}
	cacheStore := newFakeCache()
	cacheStore.lockErr = cache.ErrLockNotAcquired
	capture := newDBCaptureLogger()
	db := NewCached(base, cacheStore, capture, nil, false)

	var got testDoc
	err := db.FindOne(context.Background(), "docs", map[string]string{"id": "1"}, &got, ReadOptions{
		CacheKey:   "doc:1",
		CacheTTL:   time.Minute,
		LockOnMiss: true,
	})
	if err != nil {
		t.Fatalf("FindOne() error = %v", err)
	}
	if !capture.hasEntry("warn", "cache read lock not acquired, falling back to database") {
		t.Fatalf("logger entries = %+v", capture.entries)
	}
}

func TestCachedDatabaseFindOneMapsTimeoutToDependencyError(t *testing.T) {
	base := &fakeDatabase{findOneErr: context.DeadlineExceeded}
	capture := newDBCaptureLogger()
	registry := prometheus.NewRegistry()
	metrics, err := platformmetrics.NewDatabaseMetrics(registry, "testapp")
	if err != nil {
		t.Fatalf("NewDatabaseMetrics() error = %v", err)
	}
	db := NewCached(base, nil, capture, metrics, false)

	var got testDoc
	err = db.FindOne(context.Background(), "docs", map[string]string{"id": "1"}, &got, ReadOptions{})
	appErr := apperrors.FromError(err)
	if appErr == nil || appErr.Code != apperrors.CodeDependency {
		t.Fatalf("FindOne() error = %v", err)
	}
	if !capture.hasEntry("warn", "database dependency error") || !capture.hasField("op", "CachedDatabase.FindOne") {
		t.Fatalf("logger entries = %+v", capture.entries)
	}
	if err := testutil.GatherAndCompare(registry, strings.NewReader(`
# HELP testapp_database_dependency_errors_total Total dependency errors observed in the database abstraction.
# TYPE testapp_database_dependency_errors_total counter
testapp_database_dependency_errors_total{operation="CachedDatabase.FindOne"} 1
`), "testapp_database_dependency_errors_total"); err != nil {
		t.Fatalf("GatherAndCompare() error = %v", err)
	}
}

func TestCachedDatabaseStrictWriteLockTimeoutMapsDependencyError(t *testing.T) {
	base := &fakeDatabase{}
	cacheStore := newFakeCache()
	cacheStore.lockErr = context.DeadlineExceeded
	db := NewCached(base, cacheStore, logger.NewNoop(), nil, false)

	err := db.UpdateOne(context.Background(), "docs", nil, nil, WriteOptions{
		LockKey:    "doc:1",
		StrictLock: true,
	})
	appErr := apperrors.FromError(err)
	if appErr == nil || appErr.Code != apperrors.CodeDependency {
		t.Fatalf("UpdateOne() error = %v", err)
	}
	if base.updateCalls != 0 {
		t.Fatalf("update calls = %d", base.updateCalls)
	}
}

func TestCachedDatabaseRunInTransactionBuffersInvalidationsUntilCommit(t *testing.T) {
	base := &fakeDatabase{}
	cacheStore := newFakeCache()
	cacheStore.values["user:id:1"] = testDoc{ID: "1"}
	db := NewCached(base, cacheStore, logger.NewNoop(), nil, false)

	err := db.RunInTransaction(context.Background(), func(txCtx context.Context) error {
		if err := db.UpdateOne(txCtx, "docs", map[string]string{"id": "1"}, map[string]string{"$set": "x"}, WriteOptions{
			InvalidateKeys: []string{"user:id:1", "session:id:s1"},
			LockKey:        "user:id:1",
			StrictLock:     true,
		}); err != nil {
			return err
		}
		if base.updateCalls != 1 {
			t.Fatalf("update calls inside tx = %d", base.updateCalls)
		}
		if cacheStore.lockCalls != 0 {
			t.Fatalf("lock calls inside tx = %d", cacheStore.lockCalls)
		}
		if len(cacheStore.deleted) != 0 {
			t.Fatalf("deletions inside tx = %v", cacheStore.deleted)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunInTransaction() error = %v", err)
	}
	if len(cacheStore.deleted) != 2 {
		t.Fatalf("deletions after commit = %v", cacheStore.deleted)
	}
	if _, ok := cacheStore.values["user:id:1"]; ok {
		t.Fatal("cache key was not invalidated after commit")
	}
	if base.txCalls != 1 {
		t.Fatalf("tx calls = %d", base.txCalls)
	}
}

type fakeDatabase struct {
	findOneValue  any
	findManyValue any
	findOneErr    error
	findManyErr   error

	findOneCalls     int
	findManyCalls    int
	insertCalls      int
	updateCalls      int
	updateManyCalls  int
	deleteCalls      int
	txCalls          int
	lastFindManyOpts ReadOptions
}

func (d *fakeDatabase) FindOne(ctx context.Context, collection string, filter any, dest any, opts ReadOptions) error {
	d.findOneCalls++
	if d.findOneErr != nil {
		return d.findOneErr
	}
	return copyValue(dest, d.findOneValue)
}

func (d *fakeDatabase) FindMany(ctx context.Context, collection string, filter any, dest any, opts ReadOptions) error {
	d.findManyCalls++
	d.lastFindManyOpts = opts
	if d.findManyErr != nil {
		return d.findManyErr
	}
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

func (d *fakeDatabase) RunInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	d.txCalls++
	return fn(ctx)
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

type dbCaptureLogger struct {
	entries *[]dbLogEntry
	fields  []logger.Field
}

type dbLogEntry struct {
	level  string
	msg    string
	fields []logger.Field
}

func newDBCaptureLogger() *dbCaptureLogger {
	entries := []dbLogEntry{}
	return &dbCaptureLogger{entries: &entries}
}

func (l *dbCaptureLogger) Debug(msg string, fields ...logger.Field) {
	l.record("debug", msg, fields...)
}

func (l *dbCaptureLogger) Info(msg string, fields ...logger.Field) {
	l.record("info", msg, fields...)
}

func (l *dbCaptureLogger) Warn(msg string, fields ...logger.Field) {
	l.record("warn", msg, fields...)
}

func (l *dbCaptureLogger) Error(msg string, fields ...logger.Field) {
	l.record("error", msg, fields...)
}

func (l *dbCaptureLogger) With(fields ...logger.Field) logger.Logger {
	next := *l
	next.fields = append(append([]logger.Field{}, l.fields...), fields...)
	return &next
}

func (l *dbCaptureLogger) record(level, msg string, fields ...logger.Field) {
	entryFields := append(append([]logger.Field{}, l.fields...), fields...)
	*l.entries = append(*l.entries, dbLogEntry{level: level, msg: msg, fields: entryFields})
}

func (l *dbCaptureLogger) hasEntry(level, msg string) bool {
	for _, entry := range *l.entries {
		if entry.level == level && entry.msg == msg {
			return true
		}
	}
	return false
}

func (l *dbCaptureLogger) hasField(key string, want any) bool {
	for _, entry := range *l.entries {
		for _, field := range entry.fields {
			if field.Key == key && field.Value == want {
				return true
			}
		}
	}
	return false
}
