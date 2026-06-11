package mongo

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	"github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	"go.mongodb.org/mongo-driver/v2/bson"
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

func TestSessionRepositoryRotateRefreshTokenUsesStrictLockAndInvalidatesOldHash(t *testing.T) {
	db := &fakeDB{}
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
	wantInvalidation := []string{"session:id:s1", "session:refresh:old"}
	if !reflect.DeepEqual(db.lastWriteOptions.InvalidateKeys, wantInvalidation) {
		t.Fatalf("InvalidateKeys = %#v", db.lastWriteOptions.InvalidateKeys)
	}
}

func TestSessionRepositoryRevokeAllUsesUpdateMany(t *testing.T) {
	db := &fakeDB{}
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
}

func TestSessionRepositoryRevokeByTokenFamilyIDUsesUpdateMany(t *testing.T) {
	db := &fakeDB{}
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

	_, err := repo.List(context.Background(), auth.AuditLogFilter{ActorUserID: "u1", Action: "login"}, common.Pagination{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	filter, ok := db.lastFilter.(bson.M)
	if !ok {
		t.Fatalf("filter type = %T", db.lastFilter)
	}
	if filter["actor_user_id"] != "u1" || filter["action"] != "login" {
		t.Fatalf("filter = %#v", filter)
	}
}

type fakeDB struct {
	findOneValue  any
	findManyValue any

	lastCollection   string
	lastFilter       any
	lastUpdate       any
	lastReadOptions  database.ReadOptions
	lastWriteOptions database.WriteOptions

	insertCalls     int
	findOneCalls    int
	findManyCalls   int
	updateOneCalls  int
	updateManyCalls int
	deleteOneCalls  int
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
	return nil
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

var _ user.Repository = (*UserRepository)(nil)
var _ auth.SessionRepository = (*SessionRepository)(nil)
var _ auth.LoginHistoryRepository = (*LoginHistoryRepository)(nil)
var _ auth.AuditLogRepository = (*AuditLogRepository)(nil)
var _ auth.RevokedTokenRepository = (*RevokedTokenRepository)(nil)
