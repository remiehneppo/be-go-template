package auth

import (
	"context"
	"time"
)

type AccessClaims struct {
	UserID    string
	SessionID string
	TokenID   string
	Roles     []string
	IssuedAt  time.Time
	ExpiresAt time.Time
	KeyID     string
}

type TokenService interface {
	GenerateAccessToken(ctx context.Context, claims AccessClaims) (string, time.Time, error)
	ValidateAccessToken(ctx context.Context, token string) (*AccessClaims, error)
	GenerateRefreshToken() (plain string, hash string, err error)
	HashRefreshToken(plain string) string
	BlacklistAccessToken(ctx context.Context, tokenID string, ttl time.Duration) error
	IsAccessTokenBlacklisted(ctx context.Context, tokenID string) (bool, error)
}
