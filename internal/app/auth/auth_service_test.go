package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	"github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
	platformmetrics "github.com/remihneppo/be-go-template/internal/platform/metrics"
)

func TestNewIDFallsBackWithoutPanic(t *testing.T) {
	originalReadRand := readRand
	defer func() { readRand = originalReadRand }()
	readRand = func(b []byte) (int, error) {
		return 0, errors.New("entropy unavailable")
	}

	value := newID()
	if value == "" {
		t.Fatal("newID() returned empty string")
	}
}

func TestServiceRegisterCreatesUserSessionAndTokens(t *testing.T) {
	users := &fakeUserRepository{findErr: database.ErrNotFound}
	sessions := &fakeSessionRepository{}
	tokens := &fakeTokenService{
		refreshPlain:    "refresh",
		refreshHash:     "refresh-hash",
		accessToken:     "access",
		accessExpiresAt: time.Unix(100, 0),
		validatedClaims: &domainauth.AccessClaims{UserID: "u1", SessionID: "s1", TokenID: "jti1", ExpiresAt: time.Unix(200, 0)},
	}
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

func TestServiceRegisterUsesTransactionRunnerForCoreWrites(t *testing.T) {
	users := &fakeUserRepository{findErr: database.ErrNotFound}
	sessions := &fakeSessionRepository{}
	tokens := &fakeTokenService{
		refreshPlain:    "refresh",
		refreshHash:     "refresh-hash",
		accessToken:     "access",
		accessExpiresAt: time.Unix(100, 0),
	}
	txRunner := &fakeTransactionRunner{}
	service := NewService(ServiceDependencies{
		Users:        users,
		Sessions:     sessions,
		Tokens:       tokens,
		Passwords:    fakePasswordHasher{hash: "hashed-password"},
		RefreshTTL:   time.Hour,
		Transactions: txRunner,
	})
	service.now = func() time.Time { return time.Unix(10, 0).UTC() }

	if _, err := service.Register(context.Background(), domainauth.RegisterInput{
		Email:    "user@example.com",
		Password: "password123",
		Name:     "User",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if txRunner.calls != 1 {
		t.Fatalf("txRunner.calls = %d", txRunner.calls)
	}
	if !users.createInTx || !sessions.createInTx {
		t.Fatalf("register core writes were not in transaction: users=%v sessions=%v", users.createInTx, sessions.createInTx)
	}
}

func TestServiceRegisterMapsConflict(t *testing.T) {
	users := &fakeUserRepository{findErr: database.ErrNotFound, createErr: database.ErrConflict}
	service := NewService(ServiceDependencies{
		Users:      users,
		Sessions:   &fakeSessionRepository{},
		Tokens:     &fakeTokenService{},
		Passwords:  fakePasswordHasher{hash: "hashed-password"},
		RefreshTTL: time.Hour,
	})
	service.now = func() time.Time { return time.Unix(10, 0).UTC() }

	_, err := service.Register(context.Background(), domainauth.RegisterInput{
		Email:    "user@example.com",
		Password: "password123",
		Name:     "User",
	})
	appErr := apperrors.FromError(err)
	if appErr == nil || appErr.Code != apperrors.CodeConflict {
		t.Fatalf("Register() error = %v", err)
	}
}

func TestServiceLoginSuccessCreatesSessionAndHistory(t *testing.T) {
	users := &fakeUserRepository{found: &user.User{ID: "u1", Email: "user@example.com", PasswordHash: "hash", Roles: []user.Role{user.RoleUser}, Status: user.StatusActive}}
	sessions := &fakeSessionRepository{}
	history := &fakeLoginHistoryRepository{}
	audit := &fakeAuditLogRepository{}
	tokens := &fakeTokenService{
		refreshPlain:    "refresh",
		refreshHash:     "refresh-hash",
		accessToken:     "access",
		accessExpiresAt: time.Unix(100, 0),
		validatedClaims: &domainauth.AccessClaims{UserID: "u1", SessionID: "s1", TokenID: "jti1", ExpiresAt: time.Unix(200, 0)},
	}
	capture := newAuthCaptureLogger()
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

	ctx := logger.WithContext(context.Background(), capture)
	result, err := service.Login(ctx, domainauth.LoginInput{Email: "user@example.com", Password: "password123"}, domainauth.RequestMeta{
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
	if users.resetFailureUserID != "u1" {
		t.Fatalf("resetFailureUserID = %q", users.resetFailureUserID)
	}
	if !capture.hasEntry("info", "auth login succeeded") || !capture.hasField("user_id", "u1") || !capture.hasField("session_id", result.SessionID) {
		t.Fatalf("logger entries = %+v", capture.entries)
	}
}

func TestServiceLoginUsesTransactionRunnerForCoreWrites(t *testing.T) {
	users := &fakeUserRepository{found: &user.User{ID: "u1", Email: "user@example.com", PasswordHash: "hash", Roles: []user.Role{user.RoleUser}, Status: user.StatusActive}}
	sessions := &fakeSessionRepository{}
	history := &fakeLoginHistoryRepository{}
	tokens := &fakeTokenService{
		refreshPlain:    "refresh",
		refreshHash:     "refresh-hash",
		accessToken:     "access",
		accessExpiresAt: time.Unix(100, 0),
		validatedClaims: &domainauth.AccessClaims{UserID: "u1", SessionID: "s1", TokenID: "jti1", ExpiresAt: time.Unix(200, 0)},
	}
	txRunner := &fakeTransactionRunner{}
	service := NewService(ServiceDependencies{
		Users:        users,
		Sessions:     sessions,
		LoginHistory: history,
		Tokens:       tokens,
		Passwords:    fakePasswordHasher{},
		RefreshTTL:   time.Hour,
		Transactions: txRunner,
	})
	service.now = func() time.Time { return time.Unix(10, 0).UTC() }

	if _, err := service.Login(context.Background(), domainauth.LoginInput{Email: "user@example.com", Password: "password123"}, domainauth.RequestMeta{}); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if txRunner.calls != 1 {
		t.Fatalf("txRunner.calls = %d", txRunner.calls)
	}
	if !sessions.createInTx || !users.resetFailuresInTx || !users.updateLastLoginInTx {
		t.Fatalf("login core writes were not in transaction: sessions=%v reset=%v update=%v", sessions.createInTx, users.resetFailuresInTx, users.updateLastLoginInTx)
	}
}

func TestServiceLoginFailureWritesHistory(t *testing.T) {
	users := &fakeUserRepository{found: &user.User{ID: "u1", Email: "user@example.com", PasswordHash: "hash", Status: user.StatusActive}}
	history := &fakeLoginHistoryRepository{}
	capture := newAuthCaptureLogger()
	service := NewService(ServiceDependencies{
		Users:        users,
		Sessions:     &fakeSessionRepository{},
		LoginHistory: history,
		Tokens:       &fakeTokenService{},
		Passwords:    fakePasswordHasher{compareErr: errors.New("mismatch")},
	})

	ctx := logger.WithContext(context.Background(), capture)
	if _, err := service.Login(ctx, domainauth.LoginInput{Email: "user@example.com", Password: "password123"}, domainauth.RequestMeta{}); err == nil {
		t.Fatal("Login() error = nil")
	}
	if len(history.events) != 1 || history.events[0].Success || history.events[0].FailureReason != "invalid_credentials" {
		t.Fatalf("history = %+v", history.events)
	}
	if !capture.hasEntry("warn", "auth login failed") || !capture.hasField("reason", "invalid_credentials") {
		t.Fatalf("logger entries = %+v", capture.entries)
	}
}

func TestServiceLoginRecordsFailureAndLocksAccount(t *testing.T) {
	usr := user.New("u@example.com", "hash", "User", time.Unix(1, 0))
	usr.ID = "u1"
	usr.FailedLoginAttempts = 4
	users := &fakeUserRepository{found: &usr}
	service := NewService(ServiceDependencies{
		Users:              users,
		Sessions:           &fakeSessionRepository{},
		Tokens:             &fakeTokenService{},
		Passwords:          fakePasswordHasher{compareErr: errors.New("mismatch")},
		LockoutMaxFailures: 5,
		LockoutDuration:    15 * time.Minute,
	})
	service.now = func() time.Time { return time.Unix(100, 0).UTC() }

	_, err := service.Login(context.Background(), domainauth.LoginInput{Email: "u@example.com", Password: "wrong-password"}, domainauth.RequestMeta{})
	if err == nil {
		t.Fatal("Login() error = nil")
	}
	if users.failedAttempts != 5 {
		t.Fatalf("failedAttempts = %d", users.failedAttempts)
	}
	if users.lockedUntil == nil || !users.lockedUntil.Equal(time.Unix(100, 0).Add(15*time.Minute)) {
		t.Fatalf("lockedUntil = %v", users.lockedUntil)
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
		activeByID: &domainauth.Session{
			ID:                    "s1",
			UserID:                "u1",
			RefreshTokenHash:      "new-hash",
			RefreshTokenExpiresAt: expiresAt,
			TokenFamilyID:         "family1",
		},
		rotateErr: database.ErrNotFound,
	}
	audit := &fakeAuditLogRepository{}
	capture := newAuthCaptureLogger()
	service := NewService(ServiceDependencies{
		Users:      users,
		Sessions:   sessions,
		AuditLogs:  audit,
		Tokens:     &fakeTokenService{refreshPlain: "new-refresh", refreshHash: "new-hash"},
		Passwords:  fakePasswordHasher{},
		RefreshTTL: time.Hour,
	})
	service.now = func() time.Time { return time.Unix(10, 0).UTC() }

	ctx := logger.WithContext(context.Background(), capture)
	if _, err := service.Refresh(ctx, "old-refresh", domainauth.RequestMeta{}); err == nil {
		t.Fatal("Refresh() error = nil")
	}
	if sessions.revokedFamilyID != "family1" || sessions.revokedReason != "refresh_reuse_suspected" {
		t.Fatalf("family revocation = %q %q", sessions.revokedFamilyID, sessions.revokedReason)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "auth.refresh_reuse_suspected" {
		t.Fatalf("audit = %+v", audit.events)
	}
	if !capture.hasEntry("warn", "auth refresh reuse suspected") || !capture.hasField("token_family_id", "family1") {
		t.Fatalf("logger entries = %+v", capture.entries)
	}
}

func TestServiceRefreshDoesNotRevokeFamilyWhenSessionAlreadyInactive(t *testing.T) {
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
		rotateErr:     database.ErrNotFound,
		activeByIDErr: database.ErrNotFound,
	}
	audit := &fakeAuditLogRepository{}
	capture := newAuthCaptureLogger()
	service := NewService(ServiceDependencies{
		Users:      users,
		Sessions:   sessions,
		AuditLogs:  audit,
		Tokens:     &fakeTokenService{refreshPlain: "new-refresh", refreshHash: "new-hash"},
		Passwords:  fakePasswordHasher{},
		RefreshTTL: time.Hour,
	})
	service.now = func() time.Time { return time.Unix(10, 0).UTC() }

	ctx := logger.WithContext(context.Background(), capture)
	if _, err := service.Refresh(ctx, "old-refresh", domainauth.RequestMeta{}); err == nil {
		t.Fatal("Refresh() error = nil")
	}
	if sessions.revokedFamilyID != "" {
		t.Fatalf("family revocation = %q", sessions.revokedFamilyID)
	}
	if len(audit.events) != 0 {
		t.Fatalf("audit = %+v", audit.events)
	}
	if !capture.hasEntry("warn", "auth refresh failed") || !capture.hasField("reason", "invalid_refresh_token") {
		t.Fatalf("logger entries = %+v", capture.entries)
	}
}

func TestServiceRefreshAuditsIPAddressAnomalyByDefault(t *testing.T) {
	expiresAt := time.Unix(1000, 0).UTC()
	users := &fakeUserRepository{found: &user.User{ID: "u1", Email: "user@example.com", Roles: []user.Role{user.RoleUser}, Status: user.StatusActive}}
	sessions := &fakeSessionRepository{
		byRefreshHash: &domainauth.Session{
			ID:                    "s1",
			UserID:                "u1",
			RefreshTokenHash:      "old-hash",
			RefreshTokenExpiresAt: expiresAt,
			TokenFamilyID:         "family1",
			IP:                    "10.0.0.1",
		},
	}
	audit := &fakeAuditLogRepository{}
	capture := newAuthCaptureLogger()
	tokens := &fakeTokenService{refreshPlain: "new-refresh", refreshHash: "new-hash", accessToken: "new-access", accessExpiresAt: time.Unix(200, 0)}
	service := NewService(ServiceDependencies{
		Users:      users,
		Sessions:   sessions,
		AuditLogs:  audit,
		Tokens:     tokens,
		Passwords:  fakePasswordHasher{},
		RefreshTTL: time.Hour,
	})
	service.now = func() time.Time { return time.Unix(10, 0).UTC() }

	ctx := logger.WithContext(context.Background(), capture)
	result, err := service.Refresh(ctx, "old-refresh", domainauth.RequestMeta{IP: "10.0.0.2", UserAgent: "ua", DeviceID: "550e8400-e29b-41d4-a716-446655440000"})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if result.RefreshToken != "new-refresh" || sessions.rotatedSessionID != "s1" {
		t.Fatalf("result/rotation = %+v %q", result, sessions.rotatedSessionID)
	}
	if len(audit.events) < 2 || audit.events[0].Action != "auth.refresh_ip_anomaly" || audit.events[1].Action != "auth.refresh" {
		t.Fatalf("audit = %+v", audit.events)
	}
	if !capture.hasEntry("warn", "auth refresh ip anomaly") || !capture.hasField("action", "audit") {
		t.Fatalf("logger entries = %+v", capture.entries)
	}
}

func TestServiceRefreshRevokesOnIPAddressAnomalyWhenConfigured(t *testing.T) {
	expiresAt := time.Unix(1000, 0).UTC()
	users := &fakeUserRepository{found: &user.User{ID: "u1", Email: "user@example.com", Roles: []user.Role{user.RoleUser}, Status: user.StatusActive}}
	sessions := &fakeSessionRepository{
		byRefreshHash: &domainauth.Session{
			ID:                    "s1",
			UserID:                "u1",
			RefreshTokenHash:      "old-hash",
			RefreshTokenExpiresAt: expiresAt,
			TokenFamilyID:         "family1",
			IP:                    "10.0.0.1",
		},
	}
	audit := &fakeAuditLogRepository{}
	capture := newAuthCaptureLogger()
	service := NewService(ServiceDependencies{
		Users:                  users,
		Sessions:               sessions,
		AuditLogs:              audit,
		Tokens:                 &fakeTokenService{refreshPlain: "new-refresh", refreshHash: "new-hash", accessToken: "new-access", accessExpiresAt: time.Unix(200, 0)},
		Passwords:              fakePasswordHasher{},
		RefreshTTL:             time.Hour,
		RefreshIPAnomalyAction: "revoke",
	})
	service.now = func() time.Time { return time.Unix(10, 0).UTC() }

	ctx := logger.WithContext(context.Background(), capture)
	if _, err := service.Refresh(ctx, "old-refresh", domainauth.RequestMeta{IP: "10.0.0.2", UserAgent: "ua", DeviceID: "550e8400-e29b-41d4-a716-446655440000"}); err == nil {
		t.Fatal("Refresh() error = nil")
	}
	if sessions.revokedFamilyID != "family1" || sessions.revokedReason != "refresh_ip_anomaly" {
		t.Fatalf("revocation = %q %q", sessions.revokedFamilyID, sessions.revokedReason)
	}
	if sessions.rotatedSessionID != "" {
		t.Fatalf("rotation should not happen, got %q", sessions.rotatedSessionID)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "auth.refresh_ip_anomaly" {
		t.Fatalf("audit = %+v", audit.events)
	}
	if !capture.hasEntry("warn", "auth refresh ip anomaly") || !capture.hasField("action", "revoke") {
		t.Fatalf("logger entries = %+v", capture.entries)
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
	now := time.Unix(10, 0).UTC()
	expiresAt := now.Add(37 * time.Minute)
	tokens := &fakeTokenService{validatedClaims: &domainauth.AccessClaims{UserID: "u1", SessionID: "s1", TokenID: "jti1", ExpiresAt: expiresAt}}
	sessions := &fakeSessionRepository{}
	revoked := &fakeRevokedTokenRepository{}
	audit := &fakeAuditLogRepository{}
	capture := newAuthCaptureLogger()
	service := NewService(ServiceDependencies{
		Users:         &fakeUserRepository{},
		Sessions:      sessions,
		RevokedTokens: revoked,
		AuditLogs:     audit,
		Tokens:        tokens,
		Passwords:     fakePasswordHasher{},
	})
	service.now = func() time.Time { return now }

	ctx := logger.WithContext(context.Background(), capture)
	if err := service.Logout(ctx, "access-token", ""); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if tokens.blacklistedTokenID != "jti1" {
		t.Fatalf("blacklistedTokenID = %q", tokens.blacklistedTokenID)
	}
	wantTTL := expiresAt.Sub(now)
	if tokens.blacklistedTTL != wantTTL {
		t.Fatalf("blacklistedTTL = %s want %s", tokens.blacklistedTTL, wantTTL)
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
	if !capture.hasEntry("info", "auth logout succeeded") || !capture.hasField("session_id", "s1") || !capture.hasField("token_id", "jti1") {
		t.Fatalf("logger entries = %+v", capture.entries)
	}
}

func TestServiceRecordsAuthMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := platformmetrics.NewAuthMetrics(registry, "testapp")
	if err != nil {
		t.Fatalf("NewAuthMetrics() error = %v", err)
	}
	users := &fakeUserRepository{found: &user.User{ID: "u1", Email: "user@example.com", PasswordHash: "hash", Roles: []user.Role{user.RoleUser}, Status: user.StatusActive}}
	sessions := &fakeSessionRepository{}
	tokens := &fakeTokenService{
		refreshPlain:    "refresh",
		refreshHash:     "refresh-hash",
		accessToken:     "access",
		accessExpiresAt: time.Unix(100, 0),
		validatedClaims: &domainauth.AccessClaims{UserID: "u1", SessionID: "s1", TokenID: "jti1", ExpiresAt: time.Unix(200, 0)},
	}
	service := NewService(ServiceDependencies{
		Users:      users,
		Sessions:   sessions,
		Tokens:     tokens,
		Passwords:  fakePasswordHasher{},
		RefreshTTL: time.Hour,
		Metrics:    metrics,
	})
	service.now = func() time.Time { return time.Unix(10, 0).UTC() }

	if _, err := service.Login(logger.WithContext(context.Background(), logger.NewNoop()), domainauth.LoginInput{Email: "user@example.com", Password: "password123"}, domainauth.RequestMeta{}); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if err := service.Logout(context.Background(), "access", ""); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	if err := testutil.GatherAndCompare(registry, strings.NewReader(`
# HELP testapp_auth_login_total Total auth login attempts grouped by outcome.
# TYPE testapp_auth_login_total counter
testapp_auth_login_total{result="success"} 1
# HELP testapp_auth_logout_total Total auth logout operations handled by the API.
# TYPE testapp_auth_logout_total counter
testapp_auth_logout_total{kind="logout"} 1
# HELP testapp_auth_session_events_total Total auth session create and revoke events.
# TYPE testapp_auth_session_events_total counter
testapp_auth_session_events_total{kind="created"} 1
testapp_auth_session_events_total{kind="revoked"} 1
# HELP testapp_auth_active_sessions Current active auth sessions tracked by the API.
# TYPE testapp_auth_active_sessions gauge
testapp_auth_active_sessions 0
`), "testapp_auth_login_total", "testapp_auth_logout_total", "testapp_auth_session_events_total", "testapp_auth_active_sessions"); err != nil {
		t.Fatalf("GatherAndCompare() error = %v", err)
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
	createInTx       bool
	byRefreshHash    *domainauth.Session
	activeByID       *domainauth.Session
	activeByIDErr    error
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
	r.createInTx = transactionContextActive(ctx)
	return nil
}

func (r *fakeSessionRepository) FindActiveByID(ctx context.Context, sessionID string) (*domainauth.Session, error) {
	if r.activeByIDErr != nil {
		return nil, r.activeByIDErr
	}
	if r.activeByID != nil && r.activeByID.ID == sessionID {
		return r.activeByID, nil
	}
	return nil, database.ErrNotFound
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
	found               *user.User
	findErr             error
	created             user.User
	createErr           error
	lastLoginUserID     string
	createInTx          bool
	updateLastLoginInTx bool
	resetFailuresInTx   bool
	resetFailureUserID  string
	failedAttempts      int
	lockedUntil         *time.Time
}

func (r *fakeUserRepository) Create(ctx context.Context, usr user.User) error {
	r.created = usr
	r.createInTx = transactionContextActive(ctx)
	return r.createErr
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

func (r *fakeUserRepository) EnsureRole(ctx context.Context, userID string, role user.Role, updatedAt time.Time) error {
	return nil
}

func (r *fakeUserRepository) UpdateLastLogin(ctx context.Context, userID string, at time.Time) error {
	r.lastLoginUserID = userID
	r.updateLastLoginInTx = transactionContextActive(ctx)
	return nil
}

func (r *fakeUserRepository) RecordLoginFailure(ctx context.Context, userID string, email string, failedAttempts int, lockedUntil *time.Time, updatedAt time.Time) error {
	r.failedAttempts = failedAttempts
	r.lockedUntil = lockedUntil
	return nil
}

func (r *fakeUserRepository) ResetLoginFailures(ctx context.Context, userID string, email string, updatedAt time.Time) error {
	r.resetFailureUserID = userID
	r.resetFailuresInTx = transactionContextActive(ctx)
	return nil
}

type fakeTransactionRunner struct {
	calls int
}

func (r *fakeTransactionRunner) RunInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	r.calls++
	return fn(context.WithValue(ctx, transactionContextKey{}, true))
}

type transactionContextKey struct{}

func transactionContextActive(ctx context.Context) bool {
	active, _ := ctx.Value(transactionContextKey{}).(bool)
	return active
}

type fakeTokenService struct {
	refreshPlain       string
	refreshHash        string
	accessToken        string
	accessExpiresAt    time.Time
	lastAccessClaims   domainauth.AccessClaims
	validatedClaims    *domainauth.AccessClaims
	blacklistedTokenID string
	blacklistedTTL     time.Duration
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
	s.blacklistedTTL = ttl
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

type authCaptureLogger struct {
	entries *[]authLogEntry
	fields  []logger.Field
}

type authLogEntry struct {
	level  string
	msg    string
	fields []logger.Field
}

func newAuthCaptureLogger() *authCaptureLogger {
	entries := []authLogEntry{}
	return &authCaptureLogger{entries: &entries}
}

func (l *authCaptureLogger) Debug(msg string, fields ...logger.Field) {
	l.record("debug", msg, fields...)
}

func (l *authCaptureLogger) Info(msg string, fields ...logger.Field) {
	l.record("info", msg, fields...)
}

func (l *authCaptureLogger) Warn(msg string, fields ...logger.Field) {
	l.record("warn", msg, fields...)
}

func (l *authCaptureLogger) Error(msg string, fields ...logger.Field) {
	l.record("error", msg, fields...)
}

func (l *authCaptureLogger) With(fields ...logger.Field) logger.Logger {
	next := *l
	next.fields = append(append([]logger.Field{}, l.fields...), fields...)
	return &next
}

func (l *authCaptureLogger) record(level, msg string, fields ...logger.Field) {
	entryFields := append(append([]logger.Field{}, l.fields...), fields...)
	*l.entries = append(*l.entries, authLogEntry{level: level, msg: msg, fields: entryFields})
}

func (l *authCaptureLogger) hasEntry(level, msg string) bool {
	for _, entry := range *l.entries {
		if entry.level == level && entry.msg == msg {
			return true
		}
	}
	return false
}

func (l *authCaptureLogger) hasField(key string, want any) bool {
	for _, entry := range *l.entries {
		for _, field := range entry.fields {
			if field.Key == key && field.Value == want {
				return true
			}
		}
	}
	return false
}
