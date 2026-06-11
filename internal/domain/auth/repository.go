package auth

import (
	"context"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/common"
)

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

type LoginHistoryRepository interface {
	Append(ctx context.Context, event LoginHistory) error
	ListByUserID(ctx context.Context, userID string, pagination common.Pagination) ([]LoginHistory, error)
}

type AuditLogRepository interface {
	Append(ctx context.Context, event AuditLog) error
	List(ctx context.Context, filter AuditLogFilter, pagination common.Pagination) ([]AuditLog, error)
}

type RevokedTokenRepository interface {
	Append(ctx context.Context, token RevokedToken) error
	FindByTokenID(ctx context.Context, tokenID string) (*RevokedToken, error)
}

type AuditLogFilter struct {
	ActorUserID string
	Action      string
	RequestID   string
	From        time.Time
	To          time.Time
}

type RevokedToken struct {
	TokenID   string
	UserID    string
	SessionID string
	ExpiresAt time.Time
	RevokedAt time.Time
}
