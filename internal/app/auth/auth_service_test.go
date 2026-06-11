package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	"github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/database"
)

func TestServiceRegisterCreatesUserSessionAndTokens(t *testing.T) {
	users := &fakeUserRepository{findErr: database.ErrNotFound}
	sessions := &fakeSessionRepository{}
	tokens := &fakeTokenService{refreshPlain: "refresh", refreshHash: "refresh-hash", accessToken: "access", accessExpiresAt: time.Unix(100, 0)}
	passwords := fakePasswordHasher{hash: "hashed-password"}
	audit := &fakeAuditLogRepository{}
	service := NewService(ServiceDependencies{
		Users:      users,
		Sessions:   sessions,
		AuditLogs:  audit,
		Tokens:     tokens,
		Passwords:  passwords,
		RefreshTTL: time.Hour,
	})
	service.now = func() time.Time { return time.Unix(10, 0).UTC() }

	result, err := service.Register(context.Background(), domainauth.RegisterInput{
		Email:    " USER@Example.COM ",
		Password: "password123",
		Name:     "User",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if users.created.Email != "user@example.com" {
		t.Fatalf("created email = %q", users.created.Email)
	}
	if users.created.PasswordHash != "hashed-password" || users.created.PasswordHash == "password123" {
		t.Fatalf("created password hash = %q", users.created.PasswordHash)
	}
	if sessions.created.UserID != users.created.ID || sessions.created.RefreshTokenHash != "refresh-hash" {
		t.Fatalf("created session = %+v", sessions.created)
	}
	if result.AccessToken != "access" || result.RefreshToken != "refresh" {
		t.Fatalf("result = %+v", result)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "auth.register" {
		t.Fatalf("audit = %+v", audit.events)
	}
}

func TestServiceLoginSuccessCreatesSessionAndHistory(t *testing.T) {
	users := &fakeUserRepository{found: &user.User{ID: "u1", Email: "user@example.com", PasswordHash: "hash", Roles: []user.Role{user.RoleUser}, Status: user.StatusActive}}
	sessions := &fakeSessionRepository{}
	history := &fakeLoginHistoryRepository{}
	audit := &fakeAuditLogRepository{}
	tokens := &fakeTokenService{refreshPlain: "refresh", refreshHash: "refresh-hash", accessToken: "access", accessExpiresAt: time.Unix(100, 0)}
	service := NewService(ServiceDependencies{
		Users:        users,
		Sessions:     sessions,
		LoginHistory: history,
		AuditLogs:    audit,
		Tokens:       tokens,
		Passwords:    fakePasswordHasher{},
		RefreshTTL:   time.Hour,
	})
	service.now = func() time.Time { return time.Unix(10, 0).UTC() }

	result, err := service.Login(context.Background(), domainauth.LoginInput{Email: "user@example.com", Password: "password123"}, domainauth.RequestMeta{
		IP:         "127.0.0.1",
		UserAgent:  "test-agent",
		DeviceID:   "550e8400-e29b-41d4-a716-446655440000",
		DeviceName: "phone",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.AccessToken != "access" || result.SessionID == "" {
		t.Fatalf("result = %+v", result)
	}
	if users.lastLoginUserID != "u1" {
		t.Fatalf("lastLoginUserID = %q", users.lastLoginUserID)
	}
	if len(history.events) != 1 || !history.events[0].Success {
		t.Fatalf("history = %+v", history.events)
	}
	if sessions.created.DeviceID != "550e8400-e29b-41d4-a716-446655440000" || sessions.created.DeviceName != "phone" {
		t.Fatalf("session device = %+v", sessions.created)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "auth.login" || audit.events[0].ResourceID != result.SessionID {
		t.Fatalf("audit = %+v", audit.events)
	}
}

func TestServiceLoginFailureWritesHistory(t *testing.T) {
	users := &fakeUserRepository{found: &user.User{ID: "u1", Email: "user@example.com", PasswordHash: "hash", Status: user.StatusActive}}
	history := &fakeLoginHistoryRepository{}
	service := NewService(ServiceDependencies{
		Users:        users,
		Sessions:     &fakeSessionRepository{},
		LoginHistory: history,
		Tokens:       &fakeTokenService{},
		Passwords:    fakePasswordHasher{compareErr: errors.New("mismatch")},
	})

	if _, err := service.Login(context.Background(), domainauth.LoginInput{Email: "user@example.com", Password: "password123"}, domainauth.RequestMeta{}); err == nil {
		t.Fatal("Login() error = nil")
	}
	if len(history.events) != 1 || history.events[0].Success || history.events[0].FailureReason != "invalid_credentials" {
		t.Fatalf("history = %+v", history.events)
	}
}

func TestServiceRefreshRotatesTokenAndIssuesAccessToken(t *testing.T) {
	expiresAt := time.Unix(1000, 0).UTC()
	users := &fakeUserRepository{found: &user.User{ID: "u1", Email: "user@example.com", Roles: []user.Role{user.RoleUser}, Status: user.StatusActive}}
	sessions := &fakeSessionRepository{
		byRefreshHash: &domainauth.Session{
			ID:                    "s1",
			UserID:                "u1",
			RefreshTokenHash:      "old-hash",
			RefreshTokenExpiresAt: expiresAt,
			TokenFamilyID:         "family1",
		},
	}
	tokens := &fakeTokenService{refreshPlain: "new-refresh", refreshHash: "new-hash", accessToken: "new-access", accessExpiresAt: time.Unix(200, 0)}
	service := NewService(ServiceDependencies{
		Users:      users,
		Sessions:   sessions,
		Tokens:     tokens,
		Passwords:  fakePasswordHasher{},
		RefreshTTL: time.Hour,
	})
	service.now = func() time.Time { return time.Unix(10, 0).UTC() }

	result, err := service.Refresh(context.Background(), "old-refresh", domainauth.RequestMeta{})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if result.AccessToken != "new-access" || result.RefreshToken != "new-refresh" {
		t.Fatalf("result = %+v", result)
	}
	if sessions.rotatedSessionID != "s1" || sessions.rotatedOldHash != "old-hash" || sessions.rotatedNewHash != "new-hash" {
		t.Fatalf("rotation = %q %q %q", sessions.rotatedSessionID, sessions.rotatedOldHash, sessions.rotatedNewHash)
	}
	if tokens.lastAccessClaims.SessionID != "s1" || tokens.lastAccessClaims.UserID != "u1" {
		t.Fatalf("access claims = %+v", tokens.lastAccessClaims)
	}
}

func TestServiceRefreshReuseRevokesTokenFamilyAndAudits(t *testing.T) {
	expiresAt := time.Unix(1000, 0).UTC()
	users := &fakeUserRepository{found: &user.User{ID: "u1", Email: "user@example.com", Roles: []user.Role{user.RoleUser}, Status: user.StatusActive}}
	sessions := &fakeSessionRepository{
		byRefreshHash: &domainauth.Session{
			ID:                    "s1",
			UserID:                "u1",
			RefreshTokenHash:      "old-hash",
			RefreshTokenExpiresAt: expiresAt,
			TokenFamilyID:         "family1",
		},
		rotateErr: database.ErrNotFound,
	}
	audit := &fakeAuditLogRepository{}
	service := NewService(ServiceDependencies{
		Users:      users,
		Sessions:   sessions,
		AuditLogs:  audit,
		Tokens:     &fakeTokenService{refreshPlain: "new-refresh", refreshHash: "new-hash"},
		Passwords:  fakePasswordHasher{},
		RefreshTTL: time.Hour,
	})
	service.now = func() time.Time { return time.Unix(10, 0).UTC() }

	if _, err := service.Refresh(context.Background(), "old-refresh", domainauth.RequestMeta{}); err == nil {
		t.Fatal("Refresh() error = nil")
	}
	if sessions.revokedFamilyID != "family1" || sessions.revokedReason != "refresh_reuse_suspected" {
		t.Fatalf("family revocation = %q %q", sessions.revokedFamilyID, sessions.revokedReason)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "auth.refresh_reuse_suspected" {
		t.Fatalf("audit = %+v", audit.events)
	}
}

func TestServiceRefreshRejectsInvalidToken(t *testing.T) {
	service := NewService(ServiceDependencies{
		Users:     &fakeUserRepository{},
		Sessions:  &fakeSessionRepository{},
		Tokens:    &fakeTokenService{},
		Passwords: fakePasswordHasher{},
	})
	if _, err := service.Refresh(context.Background(), "missing", domainauth.RequestMeta{}); err == nil {
		t.Fatal("Refresh() error = nil")
	}
}

func TestServiceLogoutBlacklistsTokenAndRevokesSession(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour)
	tokens := &fakeTokenService{validatedClaims: &domainauth.AccessClaims{UserID: "u1", SessionID: "s1", TokenID: "jti1", ExpiresAt: expiresAt}}
	sessions := &fakeSessionRepository{}
	revoked := &fakeRevokedTokenRepository{}
	audit := &fakeAuditLogRepository{}
	service := NewService(ServiceDependencies{
		Users:         &fakeUserRepository{},
		Sessions:      sessions,
		RevokedTokens: revoked,
		AuditLogs:     audit,
		Tokens:        tokens,
		Passwords:     fakePasswordHasher{},
	})
	service.now = func() time.Time { return time.Unix(10, 0).UTC() }

	if err := service.Logout(context.Background(), "access-token", ""); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if tokens.blacklistedTokenID != "jti1" {
		t.Fatalf("blacklistedTokenID = %q", tokens.blacklistedTokenID)
	}
	if revoked.token.TokenID != "jti1" || revoked.token.UserID != "u1" || revoked.token.SessionID != "s1" {
		t.Fatalf("revoked token = %+v", revoked.token)
	}
	if sessions.revokedSessionID != "s1" || sessions.revokedReason != "logout" {
		t.Fatalf("revoked = %q %q", sessions.revokedSessionID, sessions.revokedReason)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "auth.logout" || audit.events[0].ResourceID != "s1" {
		t.Fatalf("audit = %+v", audit.events)
	}
}

func TestServiceListDevicesMapsActiveSessions(t *testing.T) {
	sessions := &fakeSessionRepository{
		active: []domainauth.Session{{
			ID:         "s1",
			UserID:     "u1",
			DeviceID:   "device",
			DeviceName: "phone",
			LastSeenAt: time.Unix(10, 0),
			CreatedAt:  time.Unix(1, 0),
		}},
	}
	service := NewService(ServiceDependencies{Sessions: sessions})

	got, err := service.ListDevices(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListDevices() error = %v", err)
	}
	if len(got) != 1 || got[0].SessionID != "s1" || got[0].DeviceName != "phone" {
		t.Fatalf("devices = %+v", got)
	}
}

func TestServiceLogoutAllRevokesSessions(t *testing.T) {
	sessions := &fakeSessionRepository{}
	service := NewService(ServiceDependencies{Sessions: sessions})
	service.now = func() time.Time { return time.Unix(10, 0).UTC() }

	if err := service.LogoutAll(context.Background(), "u1"); err != nil {
		t.Fatalf("LogoutAll() error = %v", err)
	}
	if sessions.revokedUserID != "u1" || sessions.revokedReason != "logout_all" {
		t.Fatalf("revocation = %q %q", sessions.revokedUserID, sessions.revokedReason)
	}
}

type fakeSessionRepository struct {
	active           []domainauth.Session
	created          domainauth.Session
	byRefreshHash    *domainauth.Session
	rotatedSessionID string
	rotatedOldHash   string
	rotatedNewHash   string
	rotateErr        error
	revokedFamilyID  string
	revokedSessionID string
	revokedUserID    string
	revokedReason    string
}

func (r *fakeSessionRepository) Create(ctx context.Context, session domainauth.Session) error {
	r.created = session
	return nil
}

func (r *fakeSessionRepository) FindActiveByID(ctx context.Context, sessionID string) (*domainauth.Session, error) {
	return nil, nil
}

func (r *fakeSessionRepository) FindByRefreshTokenHash(ctx context.Context, hash string) (*domainauth.Session, error) {
	if r.byRefreshHash == nil || r.byRefreshHash.RefreshTokenHash != hash {
		return nil, database.ErrNotFound
	}
	return r.byRefreshHash, nil
}

func (r *fakeSessionRepository) RotateRefreshToken(ctx context.Context, sessionID string, oldHash string, newHash string, expiresAt time.Time) error {
	r.rotatedSessionID = sessionID
	r.rotatedOldHash = oldHash
	r.rotatedNewHash = newHash
	return r.rotateErr
}

func (r *fakeSessionRepository) RevokeByTokenFamilyID(ctx context.Context, tokenFamilyID string, reason string, revokedAt time.Time) error {
	r.revokedFamilyID = tokenFamilyID
	r.revokedReason = reason
	return nil
}

func (r *fakeSessionRepository) Revoke(ctx context.Context, sessionID string, reason string, revokedAt time.Time) error {
	r.revokedSessionID = sessionID
	r.revokedReason = reason
	return nil
}

func (r *fakeSessionRepository) RevokeAllByUserID(ctx context.Context, userID string, reason string, revokedAt time.Time) error {
	r.revokedUserID = userID
	r.revokedReason = reason
	return nil
}

func (r *fakeSessionRepository) ListActiveByUserID(ctx context.Context, userID string) ([]domainauth.Session, error) {
	return r.active, nil
}

type fakeLoginHistoryRepository struct {
	events []domainauth.LoginHistory
}

func (r *fakeLoginHistoryRepository) Append(ctx context.Context, event domainauth.LoginHistory) error {
	r.events = append(r.events, event)
	return nil
}

func (r *fakeLoginHistoryRepository) ListByUserID(ctx context.Context, userID string, pagination common.Pagination) ([]domainauth.LoginHistory, error) {
	return r.events, nil
}

type fakeUserRepository struct {
	found           *user.User
	findErr         error
	created         user.User
	lastLoginUserID string
}

func (r *fakeUserRepository) Create(ctx context.Context, usr user.User) error {
	r.created = usr
	return nil
}

func (r *fakeUserRepository) FindByID(ctx context.Context, id string) (*user.User, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	return r.found, nil
}

func (r *fakeUserRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	return r.found, nil
}

func (r *fakeUserRepository) UpdateLastLogin(ctx context.Context, userID string, at time.Time) error {
	r.lastLoginUserID = userID
	return nil
}

type fakeTokenService struct {
	refreshPlain       string
	refreshHash        string
	accessToken        string
	accessExpiresAt    time.Time
	lastAccessClaims   domainauth.AccessClaims
	validatedClaims    *domainauth.AccessClaims
	blacklistedTokenID string
}

func (s *fakeTokenService) GenerateAccessToken(ctx context.Context, claims domainauth.AccessClaims) (string, time.Time, error) {
	s.lastAccessClaims = claims
	return s.accessToken, s.accessExpiresAt, nil
}

func (s *fakeTokenService) ValidateAccessToken(ctx context.Context, token string) (*domainauth.AccessClaims, error) {
	if s.validatedClaims == nil {
		return nil, errors.New("invalid token")
	}
	return s.validatedClaims, nil
}

func (s *fakeTokenService) GenerateRefreshToken() (plain string, hash string, err error) {
	return s.refreshPlain, s.refreshHash, nil
}

func (s *fakeTokenService) HashRefreshToken(plain string) string {
	if plain == "old-refresh" {
		return "old-hash"
	}
	return s.refreshHash
}

func (s *fakeTokenService) BlacklistAccessToken(ctx context.Context, tokenID string, ttl time.Duration) error {
	s.blacklistedTokenID = tokenID
	return nil
}

func (s *fakeTokenService) IsAccessTokenBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	return false, nil
}

type fakeAuditLogRepository struct {
	events []domainauth.AuditLog
}

func (r *fakeAuditLogRepository) Append(ctx context.Context, event domainauth.AuditLog) error {
	r.events = append(r.events, event)
	return nil
}

func (r *fakeAuditLogRepository) List(ctx context.Context, filter domainauth.AuditLogFilter, pagination common.Pagination) ([]domainauth.AuditLog, error) {
	return r.events, nil
}

type fakeRevokedTokenRepository struct {
	token domainauth.RevokedToken
}

func (r *fakeRevokedTokenRepository) Append(ctx context.Context, token domainauth.RevokedToken) error {
	r.token = token
	return nil
}

func (r *fakeRevokedTokenRepository) FindByTokenID(ctx context.Context, tokenID string) (*domainauth.RevokedToken, error) {
	if r.token.TokenID != tokenID {
		return nil, database.ErrNotFound
	}
	return &r.token, nil
}

type fakePasswordHasher struct {
	hash       string
	compareErr error
}

func (h fakePasswordHasher) Hash(password string) (string, error) {
	if h.hash != "" {
		return h.hash, nil
	}
	return "hashed-" + password, nil
}

func (h fakePasswordHasher) Compare(hash string, password string) error {
	return h.compareErr
}
