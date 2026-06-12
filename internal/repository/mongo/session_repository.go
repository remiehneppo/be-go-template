package mongo

import (
	"context"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const sessionsCollection = "sessions"

type SessionRepository struct {
	db database.Database
}

func NewSessionRepository(db database.Database) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(ctx context.Context, session auth.Session) error {
	doc := sessionDocumentFromDomain(session)
	return r.db.InsertOne(ctx, sessionsCollection, doc, database.WriteOptions{
		InvalidateKeys: []string{sessionIDKey(session.ID), sessionRefreshKey(session.RefreshTokenHash), userActiveSessionsKey(session.UserID)},
	})
}

func (r *SessionRepository) FindActiveByID(ctx context.Context, sessionID string) (*auth.Session, error) {
	var doc sessionDocument
	if err := r.db.FindOne(ctx, sessionsCollection, bson.M{
		"_id":                      sessionID,
		"revoked_at":               bson.M{"$exists": false},
		"refresh_token_expires_at": bson.M{"$gt": time.Now().UTC()},
	}, &doc, database.ReadOptions{
		CacheKey:   sessionIDKey(sessionID),
		CacheTTL:   2 * time.Minute,
		LockOnMiss: true,
	}); err != nil {
		return nil, err
	}
	session := doc.toDomain()
	return &session, nil
}

func (r *SessionRepository) FindByRefreshTokenHash(ctx context.Context, hash string) (*auth.Session, error) {
	var doc sessionDocument
	if err := r.db.FindOne(ctx, sessionsCollection, bson.M{"refresh_token_hash": hash}, &doc, database.ReadOptions{
		CacheKey:   sessionRefreshKey(hash),
		CacheTTL:   time.Minute,
		LockOnMiss: true,
	}); err != nil {
		return nil, err
	}
	session := doc.toDomain()
	return &session, nil
}

func (r *SessionRepository) RotateRefreshToken(ctx context.Context, sessionID string, oldHash string, newHash string, expiresAt time.Time) error {
	now := time.Now().UTC()
	session, err := r.findActiveByIDNoCache(ctx, sessionID)
	if err != nil {
		return err
	}
	return r.db.UpdateOne(ctx, sessionsCollection, bson.M{
		"_id":                      sessionID,
		"refresh_token_hash":       oldHash,
		"revoked_at":               bson.M{"$exists": false},
		"refresh_token_expires_at": bson.M{"$gt": now},
	}, bson.M{"$set": bson.M{
		"refresh_token_hash":       newHash,
		"refresh_token_expires_at": expiresAt,
		"last_seen_at":             now,
		"updated_at":               now,
	}}, database.WriteOptions{
		LockKey:        sessionIDKey(sessionID),
		InvalidateKeys: []string{sessionIDKey(sessionID), sessionRefreshKey(oldHash), userActiveSessionsKey(session.UserID)},
		StrictLock:     true,
	})
}

func (r *SessionRepository) Revoke(ctx context.Context, sessionID string, reason string, revokedAt time.Time) error {
	return r.db.UpdateOne(ctx, sessionsCollection, bson.M{"_id": sessionID}, bson.M{"$set": bson.M{
		"revoked_at":     revokedAt,
		"revoked_reason": reason,
		"updated_at":     revokedAt,
	}}, database.WriteOptions{
		LockKey:        sessionIDKey(sessionID),
		InvalidateKeys: []string{sessionIDKey(sessionID)},
		StrictLock:     true,
	})
}

func (r *SessionRepository) RevokeAllByUserID(ctx context.Context, userID string, reason string, revokedAt time.Time) error {
	sessions, err := r.findActiveSessionsByUserIDNoCache(ctx, userID)
	if err != nil {
		return err
	}
	invalidateKeys := []string{userActiveSessionsKey(userID)}
	for _, session := range sessions {
		invalidateKeys = append(invalidateKeys, sessionIDKey(session.ID), sessionRefreshKey(session.RefreshTokenHash))
	}
	return r.db.UpdateMany(ctx, sessionsCollection, bson.M{
		"user_id":    userID,
		"revoked_at": bson.M{"$exists": false},
	}, bson.M{"$set": bson.M{
		"revoked_at":     revokedAt,
		"revoked_reason": reason,
		"updated_at":     revokedAt,
	}}, database.WriteOptions{
		LockKey:        userActiveSessionsKey(userID),
		InvalidateKeys: uniqueStrings(invalidateKeys),
		StrictLock:     true,
	})
}

func (r *SessionRepository) RevokeByTokenFamilyID(ctx context.Context, tokenFamilyID string, reason string, revokedAt time.Time) error {
	sessions, err := r.findActiveSessionsByTokenFamilyIDNoCache(ctx, tokenFamilyID)
	if err != nil {
		return err
	}
	invalidateKeys := []string{"session:family:" + tokenFamilyID}
	for _, session := range sessions {
		invalidateKeys = append(invalidateKeys, sessionIDKey(session.ID), sessionRefreshKey(session.RefreshTokenHash), userActiveSessionsKey(session.UserID))
	}
	return r.db.UpdateMany(ctx, sessionsCollection, bson.M{
		"token_family_id": tokenFamilyID,
		"revoked_at":      bson.M{"$exists": false},
	}, bson.M{"$set": bson.M{
		"revoked_at":     revokedAt,
		"revoked_reason": reason,
		"updated_at":     revokedAt,
	}}, database.WriteOptions{
		LockKey:        "session:family:" + tokenFamilyID,
		InvalidateKeys: uniqueStrings(invalidateKeys),
		StrictLock:     true,
	})
}

func (r *SessionRepository) ListActiveByUserID(ctx context.Context, userID string) ([]auth.Session, error) {
	var docs []sessionDocument
	if err := r.db.FindMany(ctx, sessionsCollection, activeSessionsFilter{UserID: userID}, &docs, database.ReadOptions{
		CacheKey:   userActiveSessionsKey(userID),
		CacheTTL:   time.Minute,
		LockOnMiss: true,
		Sort:       bson.M{"last_seen_at": -1},
	}); err != nil {
		return nil, err
	}
	sessions := make([]auth.Session, 0, len(docs))
	for _, doc := range docs {
		sessions = append(sessions, doc.toDomain())
	}
	return sessions, nil
}

type activeSessionsFilter struct {
	UserID string
}

func (f activeSessionsFilter) CacheKeyParts() []string {
	return []string{"user_id", f.UserID, "active"}
}

func (f activeSessionsFilter) MarshalBSON() ([]byte, error) {
	return bson.Marshal(bson.M{
		"user_id":                  f.UserID,
		"revoked_at":               bson.M{"$exists": false},
		"refresh_token_expires_at": bson.M{"$gt": time.Now().UTC()},
	})
}

type tokenFamilyActiveSessionsFilter struct {
	TokenFamilyID string
}

func (f tokenFamilyActiveSessionsFilter) MarshalBSON() ([]byte, error) {
	return bson.Marshal(bson.M{
		"token_family_id":          f.TokenFamilyID,
		"revoked_at":               bson.M{"$exists": false},
		"refresh_token_expires_at": bson.M{"$gt": time.Now().UTC()},
	})
}

type sessionDocument struct {
	ID                    string     `bson:"_id" json:"id"`
	UserID                string     `bson:"user_id" json:"user_id"`
	RefreshTokenHash      string     `bson:"refresh_token_hash" json:"refresh_token_hash"`
	RefreshTokenExpiresAt time.Time  `bson:"refresh_token_expires_at" json:"refresh_token_expires_at"`
	DeviceID              string     `bson:"device_id" json:"device_id"`
	DeviceName            string     `bson:"device_name" json:"device_name"`
	UserAgent             string     `bson:"user_agent" json:"user_agent"`
	IP                    string     `bson:"ip" json:"ip"`
	TokenFamilyID         string     `bson:"token_family_id" json:"token_family_id"`
	RevokedAt             *time.Time `bson:"revoked_at,omitempty" json:"revoked_at,omitempty"`
	RevokedReason         string     `bson:"revoked_reason,omitempty" json:"revoked_reason,omitempty"`
	LastSeenAt            time.Time  `bson:"last_seen_at" json:"last_seen_at"`
	CreatedAt             time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt             time.Time  `bson:"updated_at" json:"updated_at"`
}

func sessionDocumentFromDomain(session auth.Session) sessionDocument {
	return sessionDocument{
		ID:                    session.ID,
		UserID:                session.UserID,
		RefreshTokenHash:      session.RefreshTokenHash,
		RefreshTokenExpiresAt: session.RefreshTokenExpiresAt,
		DeviceID:              auth.NormalizeDeviceID(session.DeviceID),
		DeviceName:            session.DeviceName,
		UserAgent:             session.UserAgent,
		IP:                    session.IP,
		TokenFamilyID:         session.TokenFamilyID,
		RevokedAt:             session.RevokedAt,
		RevokedReason:         session.RevokedReason,
		LastSeenAt:            session.LastSeenAt,
		CreatedAt:             session.CreatedAt,
		UpdatedAt:             session.UpdatedAt,
	}
}

func (d sessionDocument) toDomain() auth.Session {
	return auth.Session{
		ID:                    d.ID,
		UserID:                d.UserID,
		RefreshTokenHash:      d.RefreshTokenHash,
		RefreshTokenExpiresAt: d.RefreshTokenExpiresAt,
		DeviceID:              d.DeviceID,
		DeviceName:            d.DeviceName,
		UserAgent:             d.UserAgent,
		IP:                    d.IP,
		TokenFamilyID:         d.TokenFamilyID,
		RevokedAt:             d.RevokedAt,
		RevokedReason:         d.RevokedReason,
		LastSeenAt:            d.LastSeenAt,
		CreatedAt:             d.CreatedAt,
		UpdatedAt:             d.UpdatedAt,
	}
}

func (r *SessionRepository) findActiveByIDNoCache(ctx context.Context, sessionID string) (*sessionDocument, error) {
	var doc sessionDocument
	if err := r.db.FindOne(ctx, sessionsCollection, bson.M{
		"_id":                      sessionID,
		"revoked_at":               bson.M{"$exists": false},
		"refresh_token_expires_at": bson.M{"$gt": time.Now().UTC()},
	}, &doc, database.ReadOptions{}); err != nil {
		return nil, err
	}
	return &doc, nil
}

func (r *SessionRepository) findActiveSessionsByUserIDNoCache(ctx context.Context, userID string) ([]sessionDocument, error) {
	var docs []sessionDocument
	if err := r.db.FindMany(ctx, sessionsCollection, activeSessionsFilter{UserID: userID}, &docs, database.ReadOptions{}); err != nil {
		return nil, err
	}
	return docs, nil
}

func (r *SessionRepository) findActiveSessionsByTokenFamilyIDNoCache(ctx context.Context, tokenFamilyID string) ([]sessionDocument, error) {
	var docs []sessionDocument
	if err := r.db.FindMany(ctx, sessionsCollection, tokenFamilyActiveSessionsFilter{TokenFamilyID: tokenFamilyID}, &docs, database.ReadOptions{}); err != nil {
		return nil, err
	}
	return docs, nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
