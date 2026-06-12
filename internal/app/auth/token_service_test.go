package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/platform/cache"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
)

func TestTokenServiceGenerateAndValidateAccessToken(t *testing.T) {
	svc := newTestTokenService(t)
	svc.now = func() time.Time { return time.Unix(100, 0).UTC() }

	token, expiresAt, err := svc.GenerateAccessToken(context.Background(), domainauth.AccessClaims{
		UserID:    "u1",
		SessionID: "s1",
		TokenID:   "jti1",
		Roles:     []string{"user"},
	})
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	if !expiresAt.Equal(time.Unix(100, 0).UTC().Add(time.Minute)) {
		t.Fatalf("expiresAt = %s", expiresAt)
	}

	got, err := svc.ValidateAccessToken(context.Background(), token)
	if err != nil {
		t.Fatalf("ValidateAccessToken() error = %v", err)
	}
	if got.UserID != "u1" || got.SessionID != "s1" || got.TokenID != "jti1" || got.KeyID != "current" {
		t.Fatalf("claims = %+v", got)
	}
}

func TestTokenServiceRejectsExpiredPreviousKey(t *testing.T) {
	current := keyValue("current", "current-secret-value")
	previous := keyValue("previous", "previous-secret-value")
	svc, err := NewTokenService(TokenConfig{
		CurrentKey:       current,
		PreviousKey:      previous,
		PreviousNotAfter: time.Unix(50, 0).UTC(),
		AccessTTL:        time.Minute,
		RefreshTTL:       time.Hour,
	}, newMemoryCache(), nil)
	if err != nil {
		t.Fatalf("NewTokenService() error = %v", err)
	}
	svc.now = func() time.Time { return time.Unix(100, 0).UTC() }

	token := signTestToken(t, "previous", []byte("previous-secret-value"))
	_, err = svc.ValidateAccessToken(context.Background(), token)
	if err == nil {
		t.Fatal("ValidateAccessToken() error = nil")
	}
}

func TestTokenServiceReturnsExpiredError(t *testing.T) {
	svc := newTestTokenService(t)
	svc.now = func() time.Time { return time.Unix(200, 0).UTC() }
	token := signTestTokenAt(t, "current", []byte("current-secret-value"), time.Unix(100, 0).UTC(), time.Unix(150, 0).UTC())

	_, err := svc.ValidateAccessToken(context.Background(), token)
	if err == nil {
		t.Fatal("ValidateAccessToken() error = nil")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeTokenExpired {
		t.Fatalf("err = %v", err)
	}
}

func TestRefreshTokenHashIsStableAndDoesNotExposePlaintext(t *testing.T) {
	svc := newTestTokenService(t)
	plain, hash, err := svc.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken() error = %v", err)
	}
	if plain == "" || hash == "" {
		t.Fatalf("plain/hash empty: %q %q", plain, hash)
	}
	if plain == hash {
		t.Fatal("hash equals plaintext")
	}
	if got := svc.HashRefreshToken(plain); got != hash {
		t.Fatalf("HashRefreshToken() = %q, want %q", got, hash)
	}
}

func TestBlacklistAccessTokenUsesCache(t *testing.T) {
	cacheStore := newMemoryCache()
	svc := newTestTokenService(t)
	svc.cache = cacheStore

	if err := svc.BlacklistAccessToken(context.Background(), "jti1", time.Minute); err != nil {
		t.Fatalf("BlacklistAccessToken() error = %v", err)
	}
	got, err := svc.IsAccessTokenBlacklisted(context.Background(), "jti1")
	if err != nil {
		t.Fatalf("IsAccessTokenBlacklisted() error = %v", err)
	}
	if !got {
		t.Fatal("IsAccessTokenBlacklisted() = false")
	}
}

func TestTokenServiceReturnsRevokedError(t *testing.T) {
	cacheStore := newMemoryCache()
	repo := &fakeRevokedRepo{token: &domainauth.RevokedToken{
		TokenID:   "jti1",
		ExpiresAt: time.Now().Add(time.Hour),
		RevokedAt: time.Now(),
	}}
	svc := newTestTokenService(t)
	svc.cache = cacheStore
	svc.revoked = repo

	_, err := svc.ValidateAccessToken(context.Background(), signTestTokenAt(t, "current", []byte("current-secret-value"), time.Now().Add(-time.Minute), time.Now().Add(time.Minute)))
	if err == nil {
		t.Fatal("ValidateAccessToken() error = nil")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeTokenRevoked {
		t.Fatalf("err = %v", err)
	}
}

func TestBlacklistFallsBackToRevokedRepositoryOnCacheMiss(t *testing.T) {
	cacheStore := newMemoryCache()
	repo := &fakeRevokedRepo{token: &domainauth.RevokedToken{
		TokenID:   "jti1",
		ExpiresAt: time.Now().Add(time.Hour),
		RevokedAt: time.Now(),
	}}
	svc := newTestTokenService(t)
	svc.cache = cacheStore
	svc.revoked = repo

	got, err := svc.IsAccessTokenBlacklisted(context.Background(), "jti1")
	if err != nil {
		t.Fatalf("IsAccessTokenBlacklisted() error = %v", err)
	}
	if !got {
		t.Fatal("IsAccessTokenBlacklisted() = false")
	}
	if repo.calls != 1 {
		t.Fatalf("repo calls = %d", repo.calls)
	}
}

func TestBlacklistWarmsCacheFromRevokedRepository(t *testing.T) {
	cacheStore := newMemoryCache()
	repo := &fakeRevokedRepo{token: &domainauth.RevokedToken{
		TokenID:   "jti1",
		ExpiresAt: time.Now().Add(time.Hour),
		RevokedAt: time.Now(),
	}}
	svc := newTestTokenService(t)
	svc.cache = cacheStore
	svc.revoked = repo

	got, err := svc.IsAccessTokenBlacklisted(context.Background(), "jti1")
	if err != nil {
		t.Fatalf("IsAccessTokenBlacklisted() error = %v", err)
	}
	if !got {
		t.Fatal("IsAccessTokenBlacklisted() = false")
	}
	value, ok := cacheStore.values[accessBlacklistPrefix+"jti1"]
	if !ok || value != true {
		t.Fatalf("cache value = %#v ok=%v", value, ok)
	}
}

func TestParseJWTKeyRejectsBadFormat(t *testing.T) {
	if _, err := ParseJWTKey("missing-separator", time.Time{}); err == nil {
		t.Fatal("ParseJWTKey() error = nil")
	}
}

func newTestTokenService(t *testing.T) *TokenService {
	t.Helper()
	svc, err := NewTokenService(TokenConfig{
		CurrentKey: keyValue("current", "current-secret-value"),
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
	}, newMemoryCache(), nil)
	if err != nil {
		t.Fatalf("NewTokenService() error = %v", err)
	}
	return svc
}

func keyValue(id string, secret string) string {
	return id + "/" + base64.RawURLEncoding.EncodeToString([]byte(secret))
}

func signTestToken(t *testing.T, kid string, secret []byte) string {
	t.Helper()
	return signTestTokenAt(t, kid, secret, time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
}

func signTestTokenAt(t *testing.T, kid string, secret []byte, issuedAt time.Time, expiresAt time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":        "u1",
		"session_id": "s1",
		"jti":        "jti1",
		"roles":      []string{"user"},
		"iat":        issuedAt.Unix(),
		"exp":        expiresAt.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	return signed
}

type fakeRevokedRepo struct {
	token *domainauth.RevokedToken
	calls int
}

func (r *fakeRevokedRepo) Append(ctx context.Context, token domainauth.RevokedToken) error {
	r.token = &token
	return nil
}

func (r *fakeRevokedRepo) FindByTokenID(ctx context.Context, tokenID string) (*domainauth.RevokedToken, error) {
	r.calls++
	if r.token == nil || r.token.TokenID != tokenID {
		return nil, database.ErrNotFound
	}
	return r.token, nil
}

type memoryCache struct {
	values map[string]any
	err    error
}

func newMemoryCache() *memoryCache {
	return &memoryCache{values: map[string]any{}}
}

func (c *memoryCache) Get(ctx context.Context, key string, dest any) error {
	if c.err != nil {
		return c.err
	}
	value, ok := c.values[key]
	if !ok {
		return cache.ErrCacheMiss
	}
	switch target := dest.(type) {
	case *bool:
		*target = value.(bool)
	default:
		return errors.New("unsupported memory cache destination")
	}
	return nil
}

func (c *memoryCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	c.values[key] = value
	return nil
}

func (c *memoryCache) Delete(ctx context.Context, keys ...string) error {
	for _, key := range keys {
		delete(c.values, key)
	}
	return nil
}

func (c *memoryCache) Exists(ctx context.Context, key string) (bool, error) {
	_, ok := c.values[key]
	return ok, nil
}

func (c *memoryCache) Increment(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	return 1, nil
}

func (c *memoryCache) WithLock(ctx context.Context, key string, ttl time.Duration, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func (c *memoryCache) Ping(ctx context.Context) error {
	return nil
}

func (c *memoryCache) Close() error {
	return nil
}
