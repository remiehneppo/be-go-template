package mongo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	"github.com/remihneppo/be-go-template/internal/domain/monitoring"
	"github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/cache"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
)

func TestUserRepositoryFindByEmailNormalizesAndCaches(t *testing.T) {
	db := &fakeDB{findOneValue: userDocument{ID: "u1", Email: "user@example.com"}}
	repo := NewUserRepository(db)

	got, err := repo.FindByEmail(context.Background(), " USER@Example.COM ")
	if err != nil {
		t.Fatalf("FindByEmail() error = %v", err)
	}
	if got.Email != "user@example.com" {
		t.Fatalf("Email = %q", got.Email)
	}
	if !reflect.DeepEqual(db.lastFilter, bson.M{"email": "user@example.com"}) {
		t.Fatalf("filter = %#v", db.lastFilter)
	}
	if db.lastReadOptions.CacheKey != "user:email:user@example.com" || !db.lastReadOptions.LockOnMiss {
		t.Fatalf("read options = %+v", db.lastReadOptions)
	}
}

func TestUserRepositoryUpdateLastLoginInvalidatesID(t *testing.T) {
	db := &fakeDB{}
	repo := NewUserRepository(db)
	at := time.Unix(10, 0)

	if err := repo.UpdateLastLogin(context.Background(), "u1", at); err != nil {
		t.Fatalf("UpdateLastLogin() error = %v", err)
	}
	if db.updateOneCalls != 1 {
		t.Fatalf("updateOneCalls = %d", db.updateOneCalls)
	}
	if !reflect.DeepEqual(db.lastWriteOptions.InvalidateKeys, []string{"user:id:u1"}) {
		t.Fatalf("InvalidateKeys = %#v", db.lastWriteOptions.InvalidateKeys)
	}
}

func TestUserRepositoryUpdateLastLoginInvalidatesCachedLookup(t *testing.T) {
	base := &fakeDB{}
	cacheStore := newRepositoryCache()
	cacheStore.values[userIDKey("u1")] = userDocument{ID: "u1", Email: "user@example.com"}
	db := database.NewCached(base, cacheStore, logger.NewNoop(), nil, false)
	repo := NewUserRepository(db)
	at := time.Unix(10, 0)

	if err := repo.UpdateLastLogin(context.Background(), "u1", at); err != nil {
		t.Fatalf("UpdateLastLogin() error = %v", err)
	}
	if cacheStore.exists(userIDKey("u1")) {
		t.Fatalf("user cache key still present: %+v", cacheStore.values)
	}
}

func TestUserRepositoryEnsureRoleUsesAddToSet(t *testing.T) {
	db := &fakeDB{}
	repo := NewUserRepository(db)
	at := time.Unix(20, 0)

	if err := repo.EnsureRole(context.Background(), "u1", user.RoleAdmin, at); err != nil {
		t.Fatalf("EnsureRole() error = %v", err)
	}
	if db.updateOneCalls != 1 {
		t.Fatalf("updateOneCalls = %d", db.updateOneCalls)
	}
	update, ok := db.lastUpdate.(bson.M)
	if !ok {
		t.Fatalf("update type = %T", db.lastUpdate)
	}
	addToSet, ok := update["$addToSet"].(bson.M)
	if !ok || addToSet["roles"] != user.RoleAdmin {
		t.Fatalf("update = %#v", update)
	}
	if db.lastWriteOptions.LockKey != "user:id:u1" {
		t.Fatalf("LockKey = %q", db.lastWriteOptions.LockKey)
	}
}

func TestUserRepositoryRecordLoginFailureInvalidatesIDAndEmail(t *testing.T) {
	db := &fakeDB{}
	repo := NewUserRepository(db)
	at := time.Unix(30, 0)
	lockedUntil := time.Unix(900, 0)

	if err := repo.RecordLoginFailure(context.Background(), "u1", "USER@Example.COM", 5, &lockedUntil, at); err != nil {
		t.Fatalf("RecordLoginFailure() error = %v", err)
	}
	update, ok := db.lastUpdate.(bson.M)
	if !ok {
		t.Fatalf("update type = %T", db.lastUpdate)
	}
	set, ok := update["$set"].(bson.M)
	if !ok || set["failed_login_attempts"] != 5 || set["locked_until"] != lockedUntil {
		t.Fatalf("update = %#v", update)
	}
	if !reflect.DeepEqual(db.lastWriteOptions.InvalidateKeys, []string{"user:id:u1", "user:email:user@example.com"}) {
		t.Fatalf("InvalidateKeys = %#v", db.lastWriteOptions.InvalidateKeys)
	}
}

func TestUserRepositoryResetLoginFailuresUnsetsLock(t *testing.T) {
	db := &fakeDB{}
	repo := NewUserRepository(db)
	at := time.Unix(40, 0)

	if err := repo.ResetLoginFailures(context.Background(), "u1", "user@example.com", at); err != nil {
		t.Fatalf("ResetLoginFailures() error = %v", err)
	}
	update, ok := db.lastUpdate.(bson.M)
	if !ok {
		t.Fatalf("update type = %T", db.lastUpdate)
	}
	if _, ok := update["$unset"].(bson.M); !ok {
		t.Fatalf("update = %#v", update)
	}
}

func TestUserRepositoryCreateMapsDuplicateKeyToConflict(t *testing.T) {
	db := &fakeDB{insertErr: mongodrv.WriteException{WriteErrors: mongodrv.WriteErrors{{Code: 11000, Message: "duplicate key"}}}}
	repo := NewUserRepository(db)

	err := repo.Create(context.Background(), user.User{ID: "u1", Email: "user@example.com"})
	if !errors.Is(err, database.ErrConflict) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestSessionRepositoryRotateRefreshTokenUsesStrictLockAndInvalidatesOldHash(t *testing.T) {
	db := &fakeDB{findOneValue: sessionDocument{ID: "s1", UserID: "u1"}}
	repo := NewSessionRepository(db)

	err := repo.RotateRefreshToken(context.Background(), "s1", "old", "new", time.Unix(100, 0))
	if err != nil {
		t.Fatalf("RotateRefreshToken() error = %v", err)
	}
	if db.updateOneCalls != 1 {
		t.Fatalf("updateOneCalls = %d", db.updateOneCalls)
	}
	if !db.lastWriteOptions.StrictLock {
		t.Fatal("StrictLock = false")
	}
	if db.lastWriteOptions.LockKey != "session:id:s1" {
		t.Fatalf("LockKey = %q", db.lastWriteOptions.LockKey)
	}
	wantInvalidation := []string{"session:id:s1", "session:refresh:old", "session:user:u1:active"}
	if !reflect.DeepEqual(db.lastWriteOptions.InvalidateKeys, wantInvalidation) {
		t.Fatalf("InvalidateKeys = %#v", db.lastWriteOptions.InvalidateKeys)
	}
}

func TestSessionRepositoryRevokeAllUsesUpdateMany(t *testing.T) {
	db := &fakeDB{findManyValue: []sessionDocument{{ID: "s1", UserID: "u1", RefreshTokenHash: "old1"}, {ID: "s2", UserID: "u1", RefreshTokenHash: "old2"}}}
	repo := NewSessionRepository(db)

	if err := repo.RevokeAllByUserID(context.Background(), "u1", "logout_all", time.Unix(10, 0)); err != nil {
		t.Fatalf("RevokeAllByUserID() error = %v", err)
	}
	if db.updateManyCalls != 1 {
		t.Fatalf("updateManyCalls = %d", db.updateManyCalls)
	}
	if db.lastWriteOptions.LockKey != "session:user:u1:active" || !db.lastWriteOptions.StrictLock {
		t.Fatalf("write options = %+v", db.lastWriteOptions)
	}
	wantInvalidation := []string{"session:user:u1:active", "session:id:s1", "session:refresh:old1", "session:id:s2", "session:refresh:old2"}
	if !reflect.DeepEqual(db.lastWriteOptions.InvalidateKeys, wantInvalidation) {
		t.Fatalf("InvalidateKeys = %#v", db.lastWriteOptions.InvalidateKeys)
	}
}

func TestSessionRepositoryRevokeByTokenFamilyIDUsesUpdateMany(t *testing.T) {
	db := &fakeDB{findManyValue: []sessionDocument{{ID: "s1", UserID: "u1", RefreshTokenHash: "old1"}}}
	repo := NewSessionRepository(db)

	if err := repo.RevokeByTokenFamilyID(context.Background(), "family1", "reuse", time.Unix(10, 0)); err != nil {
		t.Fatalf("RevokeByTokenFamilyID() error = %v", err)
	}
	if db.updateManyCalls != 1 {
		t.Fatalf("updateManyCalls = %d", db.updateManyCalls)
	}
	if db.lastWriteOptions.LockKey != "session:family:family1" || !db.lastWriteOptions.StrictLock {
		t.Fatalf("write options = %+v", db.lastWriteOptions)
	}
	wantInvalidation := []string{"session:family:family1", "session:id:s1", "session:refresh:old1", "session:user:u1:active"}
	if !reflect.DeepEqual(db.lastWriteOptions.InvalidateKeys, wantInvalidation) {
		t.Fatalf("InvalidateKeys = %#v", db.lastWriteOptions.InvalidateKeys)
	}
}

func TestSessionRepositoryRevokeInvalidatesCachedSession(t *testing.T) {
	base := &fakeDB{}
	cacheStore := newRepositoryCache()
	cacheStore.values[sessionIDKey("s1")] = sessionDocument{ID: "s1", UserID: "u1", RefreshTokenHash: "old"}
	db := database.NewCached(base, cacheStore, logger.NewNoop(), nil, false)
	repo := NewSessionRepository(db)

	if err := repo.Revoke(context.Background(), "s1", "logout", time.Unix(10, 0)); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if cacheStore.exists(sessionIDKey("s1")) {
		t.Fatalf("session cache key still present: %+v", cacheStore.values)
	}
}

func TestSessionRepositoryListActiveUsesCacheableFilter(t *testing.T) {
	db := &fakeDB{findManyValue: []sessionDocument{{ID: "s1", UserID: "u1"}}}
	repo := NewSessionRepository(db)

	got, err := repo.ListActiveByUserID(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListActiveByUserID() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "s1" {
		t.Fatalf("sessions = %+v", got)
	}
	if _, ok := db.lastFilter.(activeSessionsFilter); !ok {
		t.Fatalf("filter type = %T", db.lastFilter)
	}
	if db.lastReadOptions.CacheKey != "session:user:u1:active" {
		t.Fatalf("CacheKey = %q", db.lastReadOptions.CacheKey)
	}
}

func TestLoginHistoryRepositoryPaginationOptions(t *testing.T) {
	db := &fakeDB{findManyValue: []loginHistoryDocument{{ID: "h1", UserID: "u1", CreatedAt: time.Unix(1, 0)}}}
	repo := NewLoginHistoryRepository(db)

	got, err := repo.ListByUserID(context.Background(), "u1", common.Pagination{Limit: 200, Offset: 3})
	if err != nil {
		t.Fatalf("ListByUserID() error = %v", err)
	}
	if len(got) != 1 || got[0].CreatedAt.IsZero() {
		t.Fatalf("history = %+v", got)
	}
	if db.lastReadOptions.Limit != 100 || db.lastReadOptions.Offset != 3 {
		t.Fatalf("read options = %+v", db.lastReadOptions)
	}
}

func TestAuditLogRepositoryBuildsFilter(t *testing.T) {
	db := &fakeDB{findManyValue: []auditLogDocument{{ID: "a1", ActorUserID: "u1", CreatedAt: time.Unix(1, 0)}}}
	repo := NewAuditLogRepository(db)

	_, err := repo.List(context.Background(), auth.AuditLogFilter{ActorUserID: "u1", Action: "login", ResourceType: "session", ResourceID: "s1"}, common.Pagination{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	filter, ok := db.lastFilter.(bson.M)
	if !ok {
		t.Fatalf("filter type = %T", db.lastFilter)
	}
	if filter["actor_user_id"] != "u1" || filter["action"] != "login" || filter["resource_type"] != "session" || filter["resource_id"] != "s1" {
		t.Fatalf("filter = %#v", filter)
	}
}

func TestMonitoringStatsRepositoryCountsAuthStats(t *testing.T) {
	db := &fakeDB{countValues: []int64{3, 2, 4, 1, 5, 6}}
	repo := NewMonitoringStatsRepository(db)
	from := time.Unix(100, 0).UTC()
	to := time.Unix(200, 0).UTC()

	got, err := repo.GetAuthStats(context.Background(), from, to)
	if err != nil {
		t.Fatalf("GetAuthStats() error = %v", err)
	}
	if got.LoginSuccessCount != 3 || got.LoginFailureCount != 2 || got.ActiveSessionCount != 4 ||
		got.RevokedSessionCount != 1 || got.RefreshCount != 5 || got.LogoutCount != 6 {
		t.Fatalf("stats = %+v", got)
	}
	if db.countCalls != 6 {
		t.Fatalf("countCalls = %d", db.countCalls)
	}
	if db.countCollections[0] != loginHistoryCollection || db.countCollections[2] != sessionsCollection || db.countCollections[4] != auditLogsCollection {
		t.Fatalf("countCollections = %v", db.countCollections)
	}
	filter, ok := db.countFilters[0].(bson.M)
	if !ok {
		t.Fatalf("first filter type = %T", db.countFilters[0])
	}
	if filter["success"] != true {
		t.Fatalf("first filter = %#v", filter)
	}
}

func TestErrorEventRepositoryAppendAndList(t *testing.T) {
	db := &fakeDB{findManyValue: []errorEventDocument{{RequestID: "req-1", ErrorCode: "INTERNAL_ERROR", Operation: "AuthService.Refresh", CreatedAt: time.Unix(1, 0)}}}
	repo := NewErrorEventRepository(db)

	if err := repo.Append(context.Background(), auth.ErrorEvent{RequestID: "req-1", ErrorCode: "INTERNAL_ERROR", Operation: "AuthService.Refresh", Path: "/x", Method: http.MethodGet, Status: 500, CreatedAt: time.Unix(1, 0)}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if db.insertCalls != 1 || db.lastCollection != errorEventsCollection {
		t.Fatalf("insertCalls = %d collection = %q", db.insertCalls, db.lastCollection)
	}

	got, err := repo.List(context.Background(), auth.ErrorEventFilter{ErrorCode: "INTERNAL_ERROR", RequestID: "req-1", Operation: "AuthService.Refresh", Status: 500}, common.Pagination{Limit: 200, Offset: 2})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].RequestID != "req-1" {
		t.Fatalf("events = %+v", got)
	}
	if got[0].Operation != "AuthService.Refresh" {
		t.Fatalf("operation = %+v", got[0])
	}
	if db.lastReadOptions.Limit != 100 || db.lastReadOptions.Offset != 2 {
		t.Fatalf("read options = %+v", db.lastReadOptions)
	}
	filter, ok := db.lastFilter.(bson.M)
	if !ok {
		t.Fatalf("filter type = %T", db.lastFilter)
	}
	if filter["error_code"] != "INTERNAL_ERROR" || filter["request_id"] != "req-1" || filter["operation"] != "AuthService.Refresh" || filter["status"] != 500 {
		t.Fatalf("filter = %#v", filter)
	}
}

type fakeDB struct {
	findOneValue  any
	findManyValue any
	insertErr     error

	lastCollection   string
	lastFilter       any
	lastUpdate       any
	lastReadOptions  database.ReadOptions
	lastWriteOptions database.WriteOptions

	insertCalls      int
	findOneCalls     int
	findManyCalls    int
	updateOneCalls   int
	updateManyCalls  int
	deleteOneCalls   int
	countCalls       int
	countValue       int64
	countValues      []int64
	countCollections []string
	countFilters     []any
}

func (d *fakeDB) FindOne(ctx context.Context, collection string, filter any, dest any, opts database.ReadOptions) error {
	d.findOneCalls++
	d.lastCollection = collection
	d.lastFilter = filter
	d.lastReadOptions = opts
	return copyInto(dest, d.findOneValue)
}

func (d *fakeDB) FindMany(ctx context.Context, collection string, filter any, dest any, opts database.ReadOptions) error {
	d.findManyCalls++
	d.lastCollection = collection
	d.lastFilter = filter
	d.lastReadOptions = opts
	return copyInto(dest, d.findManyValue)
}

func (d *fakeDB) InsertOne(ctx context.Context, collection string, document any, opts database.WriteOptions) error {
	d.insertCalls++
	d.lastCollection = collection
	d.lastWriteOptions = opts
	return d.insertErr
}

func (d *fakeDB) UpdateOne(ctx context.Context, collection string, filter any, update any, opts database.WriteOptions) error {
	d.updateOneCalls++
	d.lastCollection = collection
	d.lastFilter = filter
	d.lastUpdate = update
	d.lastWriteOptions = opts
	return nil
}

func (d *fakeDB) UpdateMany(ctx context.Context, collection string, filter any, update any, opts database.WriteOptions) error {
	d.updateManyCalls++
	d.lastCollection = collection
	d.lastFilter = filter
	d.lastUpdate = update
	d.lastWriteOptions = opts
	return nil
}

func (d *fakeDB) DeleteOne(ctx context.Context, collection string, filter any, opts database.WriteOptions) error {
	d.deleteOneCalls++
	d.lastCollection = collection
	d.lastFilter = filter
	d.lastWriteOptions = opts
	return nil
}

func (d *fakeDB) Count(ctx context.Context, collection string, filter any) (int64, error) {
	d.countCalls++
	d.lastCollection = collection
	d.lastFilter = filter
	d.countCollections = append(d.countCollections, collection)
	d.countFilters = append(d.countFilters, filter)
	if len(d.countValues) > 0 {
		value := d.countValues[0]
		d.countValues = d.countValues[1:]
		return value, nil
	}
	return d.countValue, nil
}

func (d *fakeDB) Ping(ctx context.Context) error {
	return nil
}

func (d *fakeDB) Close(ctx context.Context) error {
	return nil
}

func copyInto(dest any, src any) error {
	payload, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, dest)
}

type repositoryCache struct {
	values map[string]any
}

func newRepositoryCache() *repositoryCache {
	return &repositoryCache{values: map[string]any{}}
}

func (c *repositoryCache) Get(ctx context.Context, key string, dest any) error {
	value, ok := c.values[key]
	if !ok {
		return cache.ErrCacheMiss
	}
	return copyInto(dest, value)
}

func (c *repositoryCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	c.values[key] = value
	return nil
}

func (c *repositoryCache) Delete(ctx context.Context, keys ...string) error {
	for _, key := range keys {
		delete(c.values, key)
	}
	return nil
}

func (c *repositoryCache) Exists(ctx context.Context, key string) (bool, error) {
	_, ok := c.values[key]
	return ok, nil
}

func (c *repositoryCache) Increment(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	return 0, nil
}

func (c *repositoryCache) WithLock(ctx context.Context, key string, ttl time.Duration, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func (c *repositoryCache) Ping(ctx context.Context) error {
	return nil
}

func (c *repositoryCache) Close() error {
	return nil
}

func (c *repositoryCache) exists(key string) bool {
	_, ok := c.values[key]
	return ok
}

var _ user.Repository = (*UserRepository)(nil)
var _ auth.SessionRepository = (*SessionRepository)(nil)
var _ auth.LoginHistoryRepository = (*LoginHistoryRepository)(nil)
var _ auth.AuditLogRepository = (*AuditLogRepository)(nil)
var _ auth.RevokedTokenRepository = (*RevokedTokenRepository)(nil)
var _ monitoring.ErrorEventRepository = (*ErrorEventRepository)(nil)
