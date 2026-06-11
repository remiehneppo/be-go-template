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
	service := NewService(ServiceDependencies{
		Users:      users,
		Sessions:   sessions,
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
}

func TestServiceLoginSuccessCreatesSessionAndHistory(t *testing.T) {
	users := &fakeUserRepository{found: &user.User{ID: "u1", Email: "user@example.com", PasswordHash: "hash", Roles: []user.Role{user.RoleUser}, Status: user.StatusActive}}
	sessions := &fakeSessionRepository{}
	history := &fakeLoginHistoryRepository{}
	tokens := &fakeTokenService{refreshPlain: "refresh", refreshHash: "refresh-hash", accessToken: "access", accessExpiresAt: time.Unix(100, 0)}
	service := NewService(ServiceDependencies{
		Users:        users,
		Sessions:     sessions,
		LoginHistory: history,
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
	active        []domainauth.Session
	created       domainauth.Session
	revokedUserID string
	revokedReason string
}

func (r *fakeSessionRepository) Create(ctx context.Context, session domainauth.Session) error {
	r.created = session
	return nil
}

func (r *fakeSessionRepository) FindActiveByID(ctx context.Context, sessionID string) (*domainauth.Session, error) {
	return nil, nil
}

func (r *fakeSessionRepository) FindByRefreshTokenHash(ctx context.Context, hash string) (*domainauth.Session, error) {
	return nil, nil
}

func (r *fakeSessionRepository) RotateRefreshToken(ctx context.Context, sessionID string, oldHash string, newHash string, expiresAt time.Time) error {
	return nil
}

func (r *fakeSessionRepository) Revoke(ctx context.Context, sessionID string, reason string, revokedAt time.Time) error {
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
	refreshPlain    string
	refreshHash     string
	accessToken     string
	accessExpiresAt time.Time
}

func (s *fakeTokenService) GenerateAccessToken(ctx context.Context, claims domainauth.AccessClaims) (string, time.Time, error) {
	return s.accessToken, s.accessExpiresAt, nil
}

func (s *fakeTokenService) ValidateAccessToken(ctx context.Context, token string) (*domainauth.AccessClaims, error) {
	return nil, nil
}

func (s *fakeTokenService) GenerateRefreshToken() (plain string, hash string, err error) {
	return s.refreshPlain, s.refreshHash, nil
}

func (s *fakeTokenService) HashRefreshToken(plain string) string {
	return s.refreshHash
}

func (s *fakeTokenService) BlacklistAccessToken(ctx context.Context, tokenID string, ttl time.Duration) error {
	return nil
}

func (s *fakeTokenService) IsAccessTokenBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	return false, nil
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
