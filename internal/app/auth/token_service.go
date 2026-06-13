package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/platform/cache"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
)

const accessBlacklistPrefix = "token:blacklist:"

// JWTKey holds a single JWT signing key with its identifier and expiration.
type JWTKey struct {
	ID       string
	Secret   []byte
	NotAfter time.Time
}

// TokenConfig holds the configuration for JWT token generation and validation.
type TokenConfig struct {
	CurrentKey       string
	PreviousKey      string
	PreviousNotAfter time.Time
	AccessTTL        time.Duration
	RefreshTTL       time.Duration
}

// TokenService generates and validates JWT access tokens, manages refresh tokens,
// and handles token blacklisting with both Redis cache and MongoDB fallback.
type TokenService struct {
	current    JWTKey
	previous   *JWTKey
	accessTTL  time.Duration
	refreshTTL time.Duration
	cache      cache.Cache
	revoked    domainauth.RevokedTokenRepository
	now        func() time.Time
}

// NewTokenService creates a TokenService from the provided configuration.
// The current and previous JWT keys are parsed; an error is returned if
// either key format is invalid.
func NewTokenService(cfg TokenConfig, cacheStore cache.Cache, revoked domainauth.RevokedTokenRepository) (*TokenService, error) {
	current, err := ParseJWTKey(cfg.CurrentKey, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("parse current jwt key: %w", err)
	}
	var previous *JWTKey
	if strings.TrimSpace(cfg.PreviousKey) != "" {
		key, err := ParseJWTKey(cfg.PreviousKey, cfg.PreviousNotAfter)
		if err != nil {
			return nil, fmt.Errorf("parse previous jwt key: %w", err)
		}
		previous = &key
	}
	if cfg.AccessTTL <= 0 {
		return nil, fmt.Errorf("access ttl must be positive")
	}
	if cfg.RefreshTTL <= 0 {
		return nil, fmt.Errorf("refresh ttl must be positive")
	}
	return &TokenService{
		current:    current,
		previous:   previous,
		accessTTL:  cfg.AccessTTL,
		refreshTTL: cfg.RefreshTTL,
		cache:      cacheStore,
		revoked:    revoked,
		now:        func() time.Time { return time.Now().UTC() },
	}, nil
}

func ParseJWTKey(value string, notAfter time.Time) (JWTKey, error) {
	parts := strings.SplitN(strings.TrimSpace(value), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return JWTKey{}, fmt.Errorf("jwt key must use <key-id>/<base64-secret> format")
	}
	secret, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return JWTKey{}, fmt.Errorf("decode jwt secret: %w", err)
	}
	if len(secret) < 16 {
		return JWTKey{}, fmt.Errorf("jwt secret must be at least 16 bytes")
	}
	return JWTKey{ID: parts[0], Secret: secret, NotAfter: notAfter}, nil
}

func (s *TokenService) GenerateAccessToken(ctx context.Context, claims domainauth.AccessClaims) (string, time.Time, error) {
	now := s.now()
	expiresAt := now.Add(s.accessTTL)
	claims.IssuedAt = now
	claims.ExpiresAt = expiresAt
	claims.KeyID = s.current.ID

	tokenClaims := jwt.MapClaims{
		"sub":        claims.UserID,
		"session_id": claims.SessionID,
		"jti":        claims.TokenID,
		"roles":      claims.Roles,
		"iat":        now.Unix(),
		"exp":        expiresAt.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenClaims)
	token.Header["kid"] = s.current.ID

	signed, err := token.SignedString(s.current.Secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func (s *TokenService) ValidateAccessToken(ctx context.Context, rawToken string) (*domainauth.AccessClaims, error) {
	claims := jwt.MapClaims{}
	parser := jwt.NewParser(jwt.WithTimeFunc(s.now))
	token, err := parser.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected jwt signing method")
		}
		kid, _ := token.Header["kid"].(string)
		key, err := s.keyForID(kid)
		if err != nil {
			return nil, err
		}
		return key.Secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, apperrors.TokenExpired(err.Error())
		}
		return nil, err
	}
	if !token.Valid {
		return nil, fmt.Errorf("invalid access token")
	}

	accessClaims, err := mapAccessClaims(token.Header, claims)
	if err != nil {
		return nil, err
	}
	if blacklisted, err := s.IsAccessTokenBlacklisted(ctx, accessClaims.TokenID); err != nil {
		return nil, err
	} else if blacklisted {
		return nil, apperrors.TokenRevoked("access token revoked")
	}
	return accessClaims, nil
}

func (s *TokenService) GenerateRefreshToken() (plain string, hash string, err error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", err
	}
	plain = base64.RawURLEncoding.EncodeToString(b[:])
	return plain, s.HashRefreshToken(plain), nil
}

func (s *TokenService) HashRefreshToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func (s *TokenService) BlacklistAccessToken(ctx context.Context, tokenID string, ttl time.Duration) error {
	if tokenID == "" || ttl <= 0 || s.cache == nil {
		return nil
	}
	return s.cache.Set(ctx, accessBlacklistPrefix+tokenID, true, ttl)
}

func (s *TokenService) IsAccessTokenBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	if tokenID == "" {
		return false, nil
	}
	if s.cache != nil {
		var blacklisted bool
		err := s.cache.Get(ctx, accessBlacklistPrefix+tokenID, &blacklisted)
		if err == nil {
			return blacklisted, nil
		}
		if err != nil && !errors.Is(err, cache.ErrCacheMiss) && s.revoked == nil {
			return false, err
		}
	}
	if s.revoked == nil {
		return false, nil
	}
	revokedToken, err := s.revoked.FindByTokenID(ctx, tokenID)
	if errors.Is(err, database.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	now := s.now()
	if revokedToken.ExpiresAt.Before(now) {
		return false, nil
	}
	if s.cache != nil {
		ignoreError(s.cache.Set(ctx, accessBlacklistPrefix+tokenID, true, time.Until(revokedToken.ExpiresAt)))
	}
	return true, nil
}

func (s *TokenService) keyForID(kid string) (JWTKey, error) {
	now := s.now()
	if kid == s.current.ID {
		return s.current, nil
	}
	if s.previous != nil && kid == s.previous.ID {
		if !s.previous.NotAfter.IsZero() && now.After(s.previous.NotAfter) {
			return JWTKey{}, apperrors.TokenExpired("previous jwt key expired")
		}
		return *s.previous, nil
	}
	return JWTKey{}, fmt.Errorf("unknown jwt key id")
}

func mapAccessClaims(header map[string]any, claims jwt.MapClaims) (*domainauth.AccessClaims, error) {
	userID, _ := claims["sub"].(string)
	sessionID, _ := claims["session_id"].(string)
	tokenID, _ := claims["jti"].(string)
	if userID == "" || sessionID == "" || tokenID == "" {
		return nil, fmt.Errorf("missing required access claims")
	}
	roles := make([]string, 0)
	if rawRoles, ok := claims["roles"].([]any); ok {
		for _, rawRole := range rawRoles {
			if role, ok := rawRole.(string); ok {
				roles = append(roles, role)
			}
		}
	}
	issuedAtUnix, _ := claims["iat"].(float64)
	expiresAtUnix, _ := claims["exp"].(float64)
	kid, _ := header["kid"].(string)
	return &domainauth.AccessClaims{
		UserID:    userID,
		SessionID: sessionID,
		TokenID:   tokenID,
		Roles:     roles,
		IssuedAt:  time.Unix(int64(issuedAtUnix), 0).UTC(),
		ExpiresAt: time.Unix(int64(expiresAtUnix), 0).UTC(),
		KeyID:     kid,
	}, nil
}

var _ domainauth.TokenService = (*TokenService)(nil)
