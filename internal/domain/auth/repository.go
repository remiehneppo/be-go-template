// Package auth defines the Session, LoginHistory, RevokedToken, and AuditLog
// entity types plus their persistence contracts.
package auth

import (
	"context"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/common"
)

// SessionRepository defines the persistence contract for Session entities.
// Session tokens are stored as hashes; the plain token is never persisted.
type SessionRepository interface {
	Create(ctx context.Context, session Session) error
	FindActiveByID(ctx context.Context, sessionID string) (*Session, error)
	FindByRefreshTokenHash(ctx context.Context, hash string) (*Session, error)
	RotateRefreshToken(ctx context.Context, sessionID string, oldHash string, newHash string, expiresAt time.Time) error
	Revoke(ctx context.Context, sessionID string, reason string, revokedAt time.Time) error
	RevokeAllByUserID(ctx context.Context, userID string, reason string, revokedAt time.Time) error
	RevokeByTokenFamilyID(ctx context.Context, tokenFamilyID string, reason string, revokedAt time.Time) error
	ListActiveByUserID(ctx context.Context, userID string) ([]Session, error)
}

// LoginHistoryRepository defines the persistence contract for login history events.
// History events are best-effort: failures do not block the login flow.
type LoginHistoryRepository interface {
	Append(ctx context.Context, event LoginHistory) error
	ListByUserID(ctx context.Context, userID string, pagination common.Pagination) ([]LoginHistory, error)
}

// AuditLogRepository defines the persistence contract for audit log events.
type AuditLogRepository interface {
	Append(ctx context.Context, event AuditLog) error
	List(ctx context.Context, filter AuditLogFilter, pagination common.Pagination) ([]AuditLog, error)
}

// RevokedTokenRepository defines the persistence contract for the revoked
// token fallback collection.
type RevokedTokenRepository interface {
	Append(ctx context.Context, token RevokedToken) error
	FindByTokenID(ctx context.Context, tokenID string) (*RevokedToken, error)
}

type ErrorEventFilter struct {
	ErrorCode string
	RequestID string
	Operation string
	Status    int
	From      time.Time
	To        time.Time
}

type AuditLogFilter struct {
	ActorUserID  string
	Action       string
	ResourceType string
	ResourceID   string
	RequestID    string
	From         time.Time
	To           time.Time
}

// RevokedToken represents a revoked access token entry in the fallback store.
type RevokedToken struct {
	TokenID   string
	UserID    string
	SessionID string
	ExpiresAt time.Time
	RevokedAt time.Time
}
