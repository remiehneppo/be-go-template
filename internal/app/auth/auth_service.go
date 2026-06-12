package auth

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	"github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
)

type ServiceDependencies struct {
	Users              user.Repository
	Sessions           domainauth.SessionRepository
	LoginHistory       domainauth.LoginHistoryRepository
	AuditLogs          domainauth.AuditLogRepository
	RevokedTokens      domainauth.RevokedTokenRepository
	Tokens             domainauth.TokenService
	Passwords          PasswordHasher
	RefreshTTL         time.Duration
	LockoutMaxFailures int
	LockoutDuration    time.Duration
}

type Service struct {
	users              user.Repository
	sessions           domainauth.SessionRepository
	loginHistory       domainauth.LoginHistoryRepository
	auditLogs          domainauth.AuditLogRepository
	revokedTokens      domainauth.RevokedTokenRepository
	tokens             domainauth.TokenService
	passwords          PasswordHasher
	refreshTTL         time.Duration
	lockoutMaxFailures int
	lockoutDuration    time.Duration
	now                func() time.Time
}

func NewService(deps ServiceDependencies) *Service {
	passwords := deps.Passwords
	if passwords == nil {
		passwords = BcryptHasher{}
	}
	refreshTTL := deps.RefreshTTL
	if refreshTTL <= 0 {
		refreshTTL = 30 * 24 * time.Hour
	}
	lockoutDuration := deps.LockoutDuration
	if lockoutDuration <= 0 {
		lockoutDuration = 15 * time.Minute
	}
	return &Service{
		users:              deps.Users,
		sessions:           deps.Sessions,
		loginHistory:       deps.LoginHistory,
		auditLogs:          deps.AuditLogs,
		revokedTokens:      deps.RevokedTokens,
		tokens:             deps.Tokens,
		passwords:          passwords,
		refreshTTL:         refreshTTL,
		lockoutMaxFailures: deps.LockoutMaxFailures,
		lockoutDuration:    lockoutDuration,
		now:                func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Register(ctx context.Context, input domainauth.RegisterInput) (*domainauth.AuthResult, error) {
	if err := s.requireAuthCore("AuthService.Register"); err != nil {
		return nil, err
	}
	email := user.NormalizeEmail(input.Email)
	if err := validateCredentials(email, input.Password); err != nil {
		return nil, err
	}
	if existing, err := s.users.FindByEmail(ctx, email); err == nil && existing != nil {
		return nil, apperrors.New(apperrors.CodeConflict, "Email already exists", http.StatusConflict)
	} else if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}

	now := s.now()
	passwordHash, err := s.passwords.Hash(input.Password)
	if err != nil {
		return nil, err
	}
	usr := user.New(email, passwordHash, input.Name, now)
	usr.ID = newID()
	if err := s.users.Create(ctx, usr); err != nil {
		if errors.Is(err, database.ErrConflict) {
			return nil, apperrors.New(apperrors.CodeConflict, "Email already exists", http.StatusConflict)
		}
		return nil, err
	}
	result, err := s.issueAuthResult(ctx, usr, domainauth.RequestMeta{})
	if err != nil {
		return nil, err
	}
	s.appendAuditLog(ctx, auditEvent("auth.register", usr.ID, "user", usr.ID, domainauth.RequestMeta{}, map[string]string{"email": usr.Email}))
	return result, nil
}

func (s *Service) Login(ctx context.Context, input domainauth.LoginInput, meta domainauth.RequestMeta) (*domainauth.AuthResult, error) {
	if err := s.requireAuthCore("AuthService.Login"); err != nil {
		return nil, err
	}
	email := user.NormalizeEmail(input.Email)
	usr, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		s.appendLoginHistory(ctx, domainauth.LoginHistory{ID: newID(), Email: email, Success: false, FailureReason: "invalid_credentials", IP: meta.IP, UserAgent: meta.UserAgent, DeviceID: domainauth.NormalizeDeviceID(meta.DeviceID), CreatedAt: s.now()})
		s.appendAuditLog(ctx, auditEvent("auth.login_failed", "", "user", "", meta, map[string]string{"email": email, "reason": "invalid_credentials"}))
		return nil, invalidCredentials()
	}
	if usr.Status != user.StatusActive || usr.IsLocked(s.now()) {
		s.appendLoginHistory(ctx, domainauth.LoginHistory{ID: newID(), UserID: usr.ID, Email: email, Success: false, FailureReason: "account_unavailable", IP: meta.IP, UserAgent: meta.UserAgent, DeviceID: domainauth.NormalizeDeviceID(meta.DeviceID), CreatedAt: s.now()})
		s.appendAuditLog(ctx, auditEvent("auth.login_failed", usr.ID, "user", usr.ID, meta, map[string]string{"email": email, "reason": "account_unavailable"}))
		return nil, apperrors.New(apperrors.CodeForbidden, "Account is not available", http.StatusForbidden)
	}
	if err := s.passwords.Compare(usr.PasswordHash, input.Password); err != nil {
		now := s.now()
		lockoutUntil := s.recordLoginFailure(ctx, *usr, now)
		reason := "invalid_credentials"
		if lockoutUntil != nil {
			reason = "account_locked"
		}
		s.appendLoginHistory(ctx, domainauth.LoginHistory{ID: newID(), UserID: usr.ID, Email: email, Success: false, FailureReason: reason, IP: meta.IP, UserAgent: meta.UserAgent, DeviceID: domainauth.NormalizeDeviceID(meta.DeviceID), CreatedAt: now})
		s.appendAuditLog(ctx, auditEvent("auth.login_failed", usr.ID, "user", usr.ID, meta, map[string]string{"email": email, "reason": reason}))
		return nil, invalidCredentials()
	}
	result, err := s.issueAuthResult(ctx, *usr, meta)
	if err != nil {
		return nil, err
	}
	now := s.now()
	ignoreError(s.users.ResetLoginFailures(ctx, usr.ID, email, now))
	ignoreError(s.users.UpdateLastLogin(ctx, usr.ID, now))
	s.appendLoginHistory(ctx, domainauth.LoginHistory{ID: newID(), UserID: usr.ID, Email: email, Success: true, IP: meta.IP, UserAgent: meta.UserAgent, DeviceID: resultDeviceID(meta.DeviceID), CreatedAt: now})
	s.appendAuditLog(ctx, auditEvent("auth.login", usr.ID, "session", result.SessionID, meta, map[string]string{"email": email}))
	return result, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string, meta domainauth.RequestMeta) (*domainauth.AuthResult, error) {
	if err := s.requireAuthCore("AuthService.Refresh"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(refreshToken) == "" {
		return nil, invalidRefreshToken()
	}
	oldHash := s.tokens.HashRefreshToken(refreshToken)
	session, err := s.sessions.FindByRefreshTokenHash(ctx, oldHash)
	if err != nil || session == nil || !session.IsActive(s.now()) {
		return nil, invalidRefreshToken()
	}
	usr, err := s.users.FindByID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if usr.Status != user.StatusActive || usr.IsLocked(s.now()) {
		return nil, apperrors.New(apperrors.CodeForbidden, "Account is not available", http.StatusForbidden)
	}
	newRefreshPlain, newRefreshHash, err := s.tokens.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}
	newRefreshExpiresAt := s.now().Add(s.refreshTTL)
	if err := s.sessions.RotateRefreshToken(ctx, session.ID, oldHash, newRefreshHash, newRefreshExpiresAt); err != nil {
		if errors.Is(err, database.ErrNotFound) && session.TokenFamilyID != "" {
			ignoreError(s.sessions.RevokeByTokenFamilyID(ctx, session.TokenFamilyID, "refresh_reuse_suspected", s.now()))
			s.appendAuditLog(ctx, auditEvent("auth.refresh_reuse_suspected", usr.ID, "session", session.ID, meta, map[string]string{"token_family_id": session.TokenFamilyID}))
			return nil, invalidRefreshToken()
		}
		return nil, err
	}
	s.appendAuditLog(ctx, auditEvent("auth.refresh", usr.ID, "session", session.ID, meta, nil))
	accessToken, accessExpiresAt, err := s.tokens.GenerateAccessToken(ctx, domainauth.AccessClaims{
		UserID:    usr.ID,
		SessionID: session.ID,
		TokenID:   newID(),
		Roles:     rolesToStrings(usr.Roles),
	})
	if err != nil {
		return nil, err
	}
	return &domainauth.AuthResult{
		User:                  *usr,
		SessionID:             session.ID,
		AccessToken:           accessToken,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshToken:          newRefreshPlain,
		RefreshTokenExpiresAt: newRefreshExpiresAt,
	}, nil
}

func (s *Service) Logout(ctx context.Context, accessToken string, sessionID string) error {
	if err := s.requireAuthCore("AuthService.Logout"); err != nil {
		return err
	}
	claims, err := s.tokens.ValidateAccessToken(ctx, accessToken)
	if err != nil {
		return apperrors.New(apperrors.CodeUnauthorized, "Unauthorized", http.StatusUnauthorized)
	}
	ttl := time.Until(claims.ExpiresAt)
	if s.revokedTokens != nil && claims.TokenID != "" {
		if err := s.revokedTokens.Append(ctx, domainauth.RevokedToken{
			TokenID:   claims.TokenID,
			UserID:    claims.UserID,
			SessionID: claims.SessionID,
			ExpiresAt: claims.ExpiresAt,
			RevokedAt: s.now(),
		}); err != nil {
			return err
		}
	}
	if ttl > 0 {
		if err := s.tokens.BlacklistAccessToken(ctx, claims.TokenID, ttl); err != nil {
			return err
		}
	}
	targetSessionID := strings.TrimSpace(sessionID)
	if targetSessionID == "" {
		targetSessionID = claims.SessionID
	}
	if targetSessionID != "" {
		if err := s.sessions.Revoke(ctx, targetSessionID, "logout", s.now()); err != nil {
			return err
		}
		s.appendAuditLog(ctx, auditEvent("auth.logout", claims.UserID, "session", targetSessionID, domainauth.RequestMeta{}, nil))
	}
	return nil
}

func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	if s.sessions == nil {
		return notImplemented("AuthService.LogoutAll")
	}
	if err := s.sessions.RevokeAllByUserID(ctx, userID, "logout_all", s.now()); err != nil {
		return err
	}
	s.appendAuditLog(ctx, auditEvent("auth.logout_all", userID, "user", userID, domainauth.RequestMeta{}, nil))
	return nil
}

func (s *Service) ListDevices(ctx context.Context, userID string) ([]domainauth.DeviceSession, error) {
	if s.sessions == nil {
		return nil, notImplemented("AuthService.ListDevices")
	}
	sessions, err := s.sessions.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	devices := make([]domainauth.DeviceSession, 0, len(sessions))
	for _, session := range sessions {
		devices = append(devices, domainauth.DeviceSession{
			SessionID:  session.ID,
			DeviceID:   session.DeviceID,
			DeviceName: session.DeviceName,
			UserAgent:  session.UserAgent,
			IP:         session.IP,
			LastSeenAt: session.LastSeenAt,
			CreatedAt:  session.CreatedAt,
			RevokedAt:  session.RevokedAt,
		})
	}
	return devices, nil
}

func (s *Service) ListLoginHistory(ctx context.Context, userID string, pagination common.Pagination) ([]domainauth.LoginHistory, error) {
	if s.loginHistory == nil {
		return nil, notImplemented("AuthService.ListLoginHistory")
	}
	return s.loginHistory.ListByUserID(ctx, userID, pagination)
}

func (s *Service) requireAuthCore(op string) error {
	if s.users == nil || s.sessions == nil || s.tokens == nil || s.passwords == nil {
		return notImplemented(op)
	}
	return nil
}

func (s *Service) issueAuthResult(ctx context.Context, usr user.User, meta domainauth.RequestMeta) (*domainauth.AuthResult, error) {
	now := s.now()
	refreshPlain, refreshHash, err := s.tokens.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}
	deviceID := resultDeviceID(meta.DeviceID)
	session := domainauth.Session{
		ID:                    newID(),
		UserID:                usr.ID,
		RefreshTokenHash:      refreshHash,
		RefreshTokenExpiresAt: now.Add(s.refreshTTL),
		DeviceID:              deviceID,
		DeviceName:            strings.TrimSpace(meta.DeviceName),
		UserAgent:             meta.UserAgent,
		IP:                    meta.IP,
		TokenFamilyID:         newID(),
		LastSeenAt:            now,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return nil, err
	}
	accessToken, accessExpiresAt, err := s.tokens.GenerateAccessToken(ctx, domainauth.AccessClaims{
		UserID:    usr.ID,
		SessionID: session.ID,
		TokenID:   newID(),
		Roles:     rolesToStrings(usr.Roles),
	})
	if err != nil {
		return nil, err
	}
	return &domainauth.AuthResult{
		User:                  usr,
		SessionID:             session.ID,
		AccessToken:           accessToken,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshToken:          refreshPlain,
		RefreshTokenExpiresAt: session.RefreshTokenExpiresAt,
	}, nil
}

func (s *Service) appendLoginHistory(ctx context.Context, event domainauth.LoginHistory) {
	if s.loginHistory == nil {
		return
	}
	ignoreError(s.loginHistory.Append(ctx, event))
}

func (s *Service) appendAuditLog(ctx context.Context, event domainauth.AuditLog) {
	if s.auditLogs == nil {
		return
	}
	if event.ID == "" {
		event.ID = newID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = s.now()
	}
	ignoreError(s.auditLogs.Append(ctx, event))
}

func (s *Service) recordLoginFailure(ctx context.Context, usr user.User, now time.Time) *time.Time {
	if s.users == nil || s.lockoutMaxFailures <= 0 {
		return nil
	}
	attempts := usr.FailedLoginAttempts + 1
	var lockedUntil *time.Time
	if attempts >= s.lockoutMaxFailures {
		until := now.Add(s.lockoutDuration)
		lockedUntil = &until
	}
	ignoreError(s.users.RecordLoginFailure(ctx, usr.ID, usr.Email, attempts, lockedUntil, now))
	return lockedUntil
}

func auditEvent(action string, actorUserID string, resourceType string, resourceID string, meta domainauth.RequestMeta, metadata map[string]string) domainauth.AuditLog {
	return domainauth.AuditLog{
		ActorUserID:  actorUserID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		IP:           meta.IP,
		UserAgent:    meta.UserAgent,
		RequestID:    meta.RequestID,
		Metadata:     metadata,
	}
}

func validateCredentials(email string, password string) error {
	var details []apperrors.ValidationDetail
	if email == "" || !strings.Contains(email, "@") {
		details = append(details, apperrors.ValidationDetail{Field: "email", Reason: "invalid_format"})
	}
	if len(password) < 8 {
		details = append(details, apperrors.ValidationDetail{Field: "password", Reason: "too_short", Meta: map[string]any{"min": 8}})
	}
	if len(details) > 0 {
		return apperrors.Validation("Invalid input", details)
	}
	return nil
}

func invalidCredentials() error {
	return apperrors.New(apperrors.CodeUnauthorized, "Invalid email or password", http.StatusUnauthorized)
}

func invalidRefreshToken() error {
	return apperrors.New(apperrors.CodeUnauthorized, "Invalid refresh token", http.StatusUnauthorized)
}

func resultDeviceID(deviceID string) string {
	if normalized := domainauth.NormalizeDeviceID(deviceID); normalized != "" {
		return normalized
	}
	return newID()
}

func rolesToStrings(roles []user.Role) []string {
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		out = append(out, string(role))
	}
	return out
}

func newID() string {
	var b [16]byte
	if _, err := readRand(b[:]); err == nil {
		b[6] = (b[6] & 0x0f) | 0x40
		b[8] = (b[8] & 0x3f) | 0x80
		buf := make([]byte, 36)
		hex.Encode(buf[0:8], b[0:4])
		buf[8] = '-'
		hex.Encode(buf[9:13], b[4:6])
		buf[13] = '-'
		hex.Encode(buf[14:18], b[6:8])
		buf[18] = '-'
		hex.Encode(buf[19:23], b[8:10])
		buf[23] = '-'
		hex.Encode(buf[24:36], b[10:16])
		return string(buf)
	}
	seq := atomic.AddUint64(&idSequence, 1)
	binary.LittleEndian.PutUint64(b[:8], uint64(time.Now().UTC().UnixNano()))
	binary.LittleEndian.PutUint64(b[8:], seq)
	return hex.EncodeToString(b[:])
}

func notImplemented(op string) error {
	return apperrors.New(apperrors.CodeDependency, op+" dependencies are not configured", http.StatusServiceUnavailable)
}

var _ domainauth.Service = (*Service)(nil)

var (
	readRand   = rand.Read
	idSequence uint64
)
