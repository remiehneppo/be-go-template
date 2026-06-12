package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	appauth "github.com/remihneppo/be-go-template/internal/app/auth"
	appmonitoring "github.com/remihneppo/be-go-template/internal/app/monitoring"
	appuser "github.com/remihneppo/be-go-template/internal/app/user"
	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	domainmonitoring "github.com/remihneppo/be-go-template/internal/domain/monitoring"
	domainuser "github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/cache"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

func TestRouterAuthFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := testConfig()
	cfg.Metrics.Enabled = false
	cfg.RateLimit.AuthEnabled = false
	cfg.HTTP.CORSAllowOrigins = []string{"http://localhost:3000"}

	users := newMemoryUserRepository()
	sessions := newMemorySessionRepository()
	loginHistory := &memoryLoginHistoryRepository{}
	auditLogs := &memoryAuditLogRepository{}
	revokedTokens := &memoryRevokedTokenRepository{}
	tokenCache := newMemoryCache()
	authStats := &memoryAuthStatsRepository{loginHistory: loginHistory, sessions: sessions, audits: auditLogs}

	tokenService, err := appauth.NewTokenService(appauth.TokenConfig{
		CurrentKey:       cfg.JWT.AccessCurrentKey,
		PreviousKey:      cfg.JWT.AccessPreviousKey,
		PreviousNotAfter: cfg.JWT.PreviousNotAfter,
		AccessTTL:        15 * time.Minute,
		RefreshTTL:       24 * time.Hour,
	}, tokenCache, revokedTokens)
	if err != nil {
		t.Fatalf("NewTokenService() error = %v", err)
	}

	authService := appauth.NewService(appauth.ServiceDependencies{
		Users:         users,
		Sessions:      sessions,
		LoginHistory:  loginHistory,
		AuditLogs:     auditLogs,
		RevokedTokens: revokedTokens,
		Tokens:        tokenService,
		RefreshTTL:    24 * time.Hour,
	})
	userService := appuser.NewService(users)
	monitoringService := appmonitoring.NewService(appmonitoring.Dependencies{AuthStats: authStats})
	router := NewRouterWithDependencies(cfg, logger.NewNoop(), RouterDependencies{
		AuthService:  authService,
		UserService:  userService,
		TokenService: tokenService,
		Monitoring:   monitoringService,
		Sessions:     sessions,
	})

	registerResp := doJSONRequest(t, router, http.MethodPost, "/v1/auth/register", map[string]any{
		"email":    "user@example.com",
		"password": "password123",
		"name":     "User",
	}, nil)
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("register status = %d body = %s", registerResp.Code, registerResp.Body.String())
	}
	registerData := decodeAuthResultResponse(t, registerResp.Body.Bytes())
	if registerData.User.Email != "user@example.com" || registerData.SessionID == "" {
		t.Fatalf("register data = %+v", registerData)
	}

	meResp := doJSONRequest(t, router, http.MethodGet, "/v1/users/me", nil, map[string]string{
		"Authorization": "Bearer " + registerData.AccessToken,
	})
	if meResp.Code != http.StatusOK {
		t.Fatalf("me status = %d body = %s", meResp.Code, meResp.Body.String())
	}
	meData := decodeUserResponse(t, meResp.Body.Bytes())
	if meData.Email != "user@example.com" {
		t.Fatalf("me data = %+v", meData)
	}

	loginResp := doJSONRequest(t, router, http.MethodPost, "/v1/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": "password123",
	}, nil)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login status = %d body = %s", loginResp.Code, loginResp.Body.String())
	}
	loginData := decodeAuthResultResponse(t, loginResp.Body.Bytes())
	if loginData.SessionID == registerData.SessionID {
		t.Fatalf("expected a new session, got %q", loginData.SessionID)
	}

	refreshResp := doJSONRequest(t, router, http.MethodPost, "/v1/auth/refresh", map[string]any{
		"refresh_token": loginData.RefreshToken,
	}, nil)
	if refreshResp.Code != http.StatusOK {
		t.Fatalf("refresh status = %d body = %s", refreshResp.Code, refreshResp.Body.String())
	}
	refreshData := decodeAuthResultResponse(t, refreshResp.Body.Bytes())
	if refreshData.RefreshToken == loginData.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}

	logoutResp := doJSONRequest(t, router, http.MethodPost, "/v1/auth/logout", map[string]any{
		"session_id": loginData.SessionID,
	}, map[string]string{
		"Authorization": "Bearer " + loginData.AccessToken,
	})
	if logoutResp.Code != http.StatusOK {
		t.Fatalf("logout status = %d body = %s", logoutResp.Code, logoutResp.Body.String())
	}

	meAfterLogoutResp := doJSONRequest(t, router, http.MethodGet, "/v1/users/me", nil, map[string]string{
		"Authorization": "Bearer " + loginData.AccessToken,
	})
	if meAfterLogoutResp.Code != http.StatusUnauthorized {
		t.Fatalf("post-logout me status = %d body = %s", meAfterLogoutResp.Code, meAfterLogoutResp.Body.String())
	}

	if got := len(loginHistory.events); got == 0 {
		t.Fatal("expected login history entries")
	}
	if got := len(auditLogs.events); got == 0 {
		t.Fatal("expected audit log entries")
	}
	if got := len(sessions.activeByUser(registerData.User.ID)); got < 2 {
		t.Fatalf("expected sessions to be persisted, got %d", got)
	}

	if err := users.EnsureRole(context.Background(), registerData.User.ID, domainuser.RoleAdmin, time.Now().UTC()); err != nil {
		t.Fatalf("EnsureRole(admin) error = %v", err)
	}
	adminLoginResp := doJSONRequest(t, router, http.MethodPost, "/v1/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": "password123",
	}, nil)
	if adminLoginResp.Code != http.StatusOK {
		t.Fatalf("admin login status = %d body = %s", adminLoginResp.Code, adminLoginResp.Body.String())
	}
	adminLoginData := decodeAuthResultResponse(t, adminLoginResp.Body.Bytes())

	statsResp := doJSONRequest(t, router, http.MethodGet, "/v1/admin/monitoring/auth-stats?from=1970-01-01T00:00:00Z&to=2100-01-01T00:00:00Z", nil, map[string]string{
		"Authorization": "Bearer " + adminLoginData.AccessToken,
	})
	if statsResp.Code != http.StatusOK {
		t.Fatalf("auth-stats status = %d body = %s", statsResp.Code, statsResp.Body.String())
	}
	var statsEnvelope struct {
		Success bool `json:"success"`
		Data    struct {
			LoginSuccessCount  int64 `json:"login_success_count"`
			LogoutCount        int64 `json:"logout_count"`
			ActiveSessionCount int64 `json:"active_session_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(statsResp.Body.Bytes(), &statsEnvelope); err != nil {
		t.Fatalf("json.Unmarshal() error = %v body = %s", err, statsResp.Body.String())
	}
	if !statsEnvelope.Success || statsEnvelope.Data.LoginSuccessCount < 2 || statsEnvelope.Data.LogoutCount == 0 || statsEnvelope.Data.ActiveSessionCount == 0 {
		t.Fatalf("auth stats = %+v body = %s", statsEnvelope, statsResp.Body.String())
	}
}

func TestRouterLogoutAllInvalidatesMultipleSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := testConfig()
	cfg.Metrics.Enabled = false
	cfg.RateLimit.AuthEnabled = false
	cfg.HTTP.CORSAllowOrigins = []string{"http://localhost:3000"}

	users := newMemoryUserRepository()
	sessions := newMemorySessionRepository()
	loginHistory := &memoryLoginHistoryRepository{}
	auditLogs := &memoryAuditLogRepository{}
	revokedTokens := &memoryRevokedTokenRepository{}
	tokenCache := newMemoryCache()

	tokenService, err := appauth.NewTokenService(appauth.TokenConfig{
		CurrentKey:       cfg.JWT.AccessCurrentKey,
		PreviousKey:      cfg.JWT.AccessPreviousKey,
		PreviousNotAfter: cfg.JWT.PreviousNotAfter,
		AccessTTL:        15 * time.Minute,
		RefreshTTL:       24 * time.Hour,
	}, tokenCache, revokedTokens)
	if err != nil {
		t.Fatalf("NewTokenService() error = %v", err)
	}

	authService := appauth.NewService(appauth.ServiceDependencies{
		Users:         users,
		Sessions:      sessions,
		LoginHistory:  loginHistory,
		AuditLogs:     auditLogs,
		RevokedTokens: revokedTokens,
		Tokens:        tokenService,
		RefreshTTL:    24 * time.Hour,
	})
	userService := appuser.NewService(users)
	router := NewRouterWithDependencies(cfg, logger.NewNoop(), RouterDependencies{
		AuthService:  authService,
		UserService:  userService,
		TokenService: tokenService,
		Sessions:     sessions,
	})

	registerResp := doJSONRequest(t, router, http.MethodPost, "/v1/auth/register", map[string]any{
		"email":    "user@example.com",
		"password": "password123",
		"name":     "User",
	}, nil)
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("register status = %d body = %s", registerResp.Code, registerResp.Body.String())
	}
	registerData := decodeAuthResultResponse(t, registerResp.Body.Bytes())

	loginResp := doJSONRequest(t, router, http.MethodPost, "/v1/auth/login", map[string]any{
		"email":    "user@example.com",
		"password": "password123",
	}, nil)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login status = %d body = %s", loginResp.Code, loginResp.Body.String())
	}
	loginData := decodeAuthResultResponse(t, loginResp.Body.Bytes())

	logoutAllResp := doJSONRequest(t, router, http.MethodPost, "/v1/auth/logout-all", nil, map[string]string{
		"Authorization": "Bearer " + loginData.AccessToken,
	})
	if logoutAllResp.Code != http.StatusOK {
		t.Fatalf("logout-all status = %d body = %s", logoutAllResp.Code, logoutAllResp.Body.String())
	}

	meWithRegisterToken := doJSONRequest(t, router, http.MethodGet, "/v1/users/me", nil, map[string]string{
		"Authorization": "Bearer " + registerData.AccessToken,
	})
	if meWithRegisterToken.Code != http.StatusUnauthorized {
		t.Fatalf("register token status = %d body = %s", meWithRegisterToken.Code, meWithRegisterToken.Body.String())
	}

	meWithLoginToken := doJSONRequest(t, router, http.MethodGet, "/v1/users/me", nil, map[string]string{
		"Authorization": "Bearer " + loginData.AccessToken,
	})
	if meWithLoginToken.Code != http.StatusUnauthorized {
		t.Fatalf("login token status = %d body = %s", meWithLoginToken.Code, meWithLoginToken.Body.String())
	}

	active, err := sessions.ListActiveByUserID(context.Background(), registerData.User.ID)
	if err != nil {
		t.Fatalf("ListActiveByUserID() error = %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected no active sessions, got %d", len(active))
	}
}

func doJSONRequest(t *testing.T, router http.Handler, method string, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		payload = data
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

type authResultEnvelope struct {
	Success bool               `json:"success"`
	Data    authResultResponse `json:"data"`
}

type userResponseEnvelope struct {
	Success bool         `json:"success"`
	Data    userResponse `json:"data"`
}

func decodeAuthResultResponse(t *testing.T, body []byte) authResultResponse {
	t.Helper()
	var envelope authResultEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode auth response: %v", err)
	}
	return envelope.Data
}

func decodeUserResponse(t *testing.T, body []byte) userResponse {
	t.Helper()
	var envelope userResponseEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode user response: %v", err)
	}
	return envelope.Data
}

type memoryCache struct {
	mu     sync.Mutex
	values map[string]memoryCacheEntry
	locks  map[string]bool
}

type memoryCacheEntry struct {
	raw       []byte
	expiresAt time.Time
}

func newMemoryCache() *memoryCache {
	return &memoryCache{
		values: make(map[string]memoryCacheEntry),
		locks:  make(map[string]bool),
	}
}

func (c *memoryCache) Get(ctx context.Context, key string, dest any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.values[key]
	if !ok {
		return cache.ErrCacheMiss
	}
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		delete(c.values, key)
		return cache.ErrCacheMiss
	}
	return json.Unmarshal(entry.raw, dest)
}

func (c *memoryCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.values[key] = memoryCacheEntry{
		raw:       payload,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()
	return nil
}

func (c *memoryCache) Delete(ctx context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		delete(c.values, key)
	}
	return nil
}

func (c *memoryCache) Exists(ctx context.Context, key string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.values[key]
	if !ok {
		return false, nil
	}
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		delete(c.values, key)
		return false, nil
	}
	return true, nil
}

func (c *memoryCache) Increment(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if ttl <= 0 {
		return 0, fmt.Errorf("increment ttl must be positive")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var current int64
	if entry, ok := c.values[key]; ok {
		ignoreError(json.Unmarshal(entry.raw, &current))
	}
	current++
	payload, err := json.Marshal(current)
	if err != nil {
		return 0, err
	}
	c.values[key] = memoryCacheEntry{
		raw:       payload,
		expiresAt: time.Now().Add(ttl),
	}
	return current, nil
}

func (c *memoryCache) WithLock(ctx context.Context, key string, ttl time.Duration, fn func(ctx context.Context) error) error {
	c.mu.Lock()
	if c.locks[key] {
		c.mu.Unlock()
		return cache.ErrLockNotAcquired
	}
	c.locks[key] = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.locks, key)
		c.mu.Unlock()
	}()
	return fn(ctx)
}

func (c *memoryCache) Ping(ctx context.Context) error {
	return nil
}

func (c *memoryCache) Close() error {
	return nil
}

type memoryUserRepository struct {
	mu     sync.Mutex
	users  map[string]domainuser.User
	emails map[string]string
}

func newMemoryUserRepository() *memoryUserRepository {
	return &memoryUserRepository{
		users:  make(map[string]domainuser.User),
		emails: make(map[string]string),
	}
}

func (r *memoryUserRepository) Create(ctx context.Context, usr domainuser.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[usr.ID] = usr
	r.emails[domainuser.NormalizeEmail(usr.Email)] = usr.ID
	return nil
}

func (r *memoryUserRepository) FindByID(ctx context.Context, id string) (*domainuser.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	usr, ok := r.users[id]
	if !ok {
		return nil, database.ErrNotFound
	}
	copy := usr
	return &copy, nil
}

func (r *memoryUserRepository) FindByEmail(ctx context.Context, email string) (*domainuser.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.emails[domainuser.NormalizeEmail(email)]
	if !ok {
		return nil, database.ErrNotFound
	}
	usr := r.users[id]
	copy := usr
	return &copy, nil
}

func (r *memoryUserRepository) EnsureRole(ctx context.Context, userID string, role domainuser.Role, updatedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	usr, ok := r.users[userID]
	if !ok {
		return database.ErrNotFound
	}
	for _, current := range usr.Roles {
		if current == role {
			return nil
		}
	}
	usr.Roles = append(usr.Roles, role)
	usr.UpdatedAt = updatedAt
	r.users[userID] = usr
	return nil
}

func (r *memoryUserRepository) UpdateLastLogin(ctx context.Context, userID string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	usr, ok := r.users[userID]
	if !ok {
		return database.ErrNotFound
	}
	usr.LastLoginAt = &at
	usr.UpdatedAt = at
	r.users[userID] = usr
	return nil
}

func (r *memoryUserRepository) RecordLoginFailure(ctx context.Context, userID string, email string, failedAttempts int, lockedUntil *time.Time, updatedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	usr, ok := r.users[userID]
	if !ok {
		return database.ErrNotFound
	}
	usr.FailedLoginAttempts = failedAttempts
	usr.LockedUntil = lockedUntil
	usr.UpdatedAt = updatedAt
	r.users[userID] = usr
	return nil
}

func (r *memoryUserRepository) ResetLoginFailures(ctx context.Context, userID string, email string, updatedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	usr, ok := r.users[userID]
	if !ok {
		return database.ErrNotFound
	}
	usr.FailedLoginAttempts = 0
	usr.LockedUntil = nil
	usr.UpdatedAt = updatedAt
	r.users[userID] = usr
	return nil
}

type memorySessionRepository struct {
	mu       sync.Mutex
	sessions map[string]domainauth.Session
	byHash   map[string]string
}

func newMemorySessionRepository() *memorySessionRepository {
	return &memorySessionRepository{
		sessions: make(map[string]domainauth.Session),
		byHash:   make(map[string]string),
	}
}

func (r *memorySessionRepository) Create(ctx context.Context, session domainauth.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[session.ID] = session
	r.byHash[session.RefreshTokenHash] = session.ID
	return nil
}

func (r *memorySessionRepository) FindActiveByID(ctx context.Context, sessionID string) (*domainauth.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[sessionID]
	if !ok || !session.IsActive(time.Now().UTC()) {
		return nil, database.ErrNotFound
	}
	copy := session
	return &copy, nil
}

func (r *memorySessionRepository) FindByRefreshTokenHash(ctx context.Context, hash string) (*domainauth.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sessionID, ok := r.byHash[hash]
	if !ok {
		return nil, database.ErrNotFound
	}
	session := r.sessions[sessionID]
	copy := session
	return &copy, nil
}

func (r *memorySessionRepository) RotateRefreshToken(ctx context.Context, sessionID string, oldHash string, newHash string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[sessionID]
	if !ok || session.RefreshTokenHash != oldHash || session.RevokedAt != nil || !session.RefreshTokenExpiresAt.After(time.Now().UTC()) {
		return database.ErrNotFound
	}
	delete(r.byHash, oldHash)
	session.RefreshTokenHash = newHash
	session.RefreshTokenExpiresAt = expiresAt
	session.LastSeenAt = time.Now().UTC()
	session.UpdatedAt = time.Now().UTC()
	r.sessions[sessionID] = session
	r.byHash[newHash] = sessionID
	return nil
}

func (r *memorySessionRepository) Revoke(ctx context.Context, sessionID string, reason string, revokedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[sessionID]
	if !ok {
		return database.ErrNotFound
	}
	session.RevokedAt = &revokedAt
	session.RevokedReason = reason
	session.UpdatedAt = revokedAt
	r.sessions[sessionID] = session
	return nil
}

func (r *memorySessionRepository) RevokeAllByUserID(ctx context.Context, userID string, reason string, revokedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, session := range r.sessions {
		if session.UserID != userID || session.RevokedAt != nil {
			continue
		}
		session.RevokedAt = &revokedAt
		session.RevokedReason = reason
		session.UpdatedAt = revokedAt
		r.sessions[id] = session
	}
	return nil
}

func (r *memorySessionRepository) RevokeByTokenFamilyID(ctx context.Context, tokenFamilyID string, reason string, revokedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, session := range r.sessions {
		if session.TokenFamilyID != tokenFamilyID || session.RevokedAt != nil {
			continue
		}
		session.RevokedAt = &revokedAt
		session.RevokedReason = reason
		session.UpdatedAt = revokedAt
		r.sessions[id] = session
	}
	return nil
}

func (r *memorySessionRepository) ListActiveByUserID(ctx context.Context, userID string) ([]domainauth.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var sessions []domainauth.Session
	for _, session := range r.sessions {
		if session.UserID != userID || session.RevokedAt != nil || !session.RefreshTokenExpiresAt.After(time.Now().UTC()) {
			continue
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (r *memorySessionRepository) activeByUser(userID string) []domainauth.Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	var sessions []domainauth.Session
	for _, session := range r.sessions {
		if session.UserID != userID {
			continue
		}
		sessions = append(sessions, session)
	}
	return sessions
}

type memoryLoginHistoryRepository struct {
	mu     sync.Mutex
	events []domainauth.LoginHistory
}

func (r *memoryLoginHistoryRepository) Append(ctx context.Context, event domainauth.LoginHistory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func (r *memoryLoginHistoryRepository) ListByUserID(ctx context.Context, userID string, pagination common.Pagination) ([]domainauth.LoginHistory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var events []domainauth.LoginHistory
	for _, event := range r.events {
		if event.UserID == userID {
			events = append(events, event)
		}
	}
	return events, nil
}

type memoryAuditLogRepository struct {
	mu     sync.Mutex
	events []domainauth.AuditLog
}

func (r *memoryAuditLogRepository) Append(ctx context.Context, event domainauth.AuditLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func (r *memoryAuditLogRepository) List(ctx context.Context, filter domainauth.AuditLogFilter, pagination common.Pagination) ([]domainauth.AuditLog, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domainauth.AuditLog(nil), r.events...), nil
}

type memoryRevokedTokenRepository struct {
	mu     sync.Mutex
	tokens map[string]domainauth.RevokedToken
}

func (r *memoryRevokedTokenRepository) Append(ctx context.Context, token domainauth.RevokedToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tokens == nil {
		r.tokens = make(map[string]domainauth.RevokedToken)
	}
	r.tokens[token.TokenID] = token
	return nil
}

func (r *memoryRevokedTokenRepository) FindByTokenID(ctx context.Context, tokenID string) (*domainauth.RevokedToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.tokens[tokenID]
	if !ok {
		return nil, database.ErrNotFound
	}
	copy := token
	return &copy, nil
}

func ignoreError(err error) {
	_ = err
}

type memoryAuthStatsRepository struct {
	loginHistory *memoryLoginHistoryRepository
	sessions     *memorySessionRepository
	audits       *memoryAuditLogRepository
}

func (r *memoryAuthStatsRepository) GetAuthStats(ctx context.Context, from time.Time, to time.Time) (*domainmonitoring.AuthStats, error) {
	r.loginHistory.mu.Lock()
	loginEvents := append([]domainauth.LoginHistory(nil), r.loginHistory.events...)
	r.loginHistory.mu.Unlock()

	r.sessions.mu.Lock()
	sessions := make([]domainauth.Session, 0, len(r.sessions.sessions))
	for _, session := range r.sessions.sessions {
		sessions = append(sessions, session)
	}
	r.sessions.mu.Unlock()

	r.audits.mu.Lock()
	audits := append([]domainauth.AuditLog(nil), r.audits.events...)
	r.audits.mu.Unlock()

	var loginSuccess, loginFailure, activeSessions, revokedSessions, refreshCount, logoutCount int64
	for _, event := range loginEvents {
		if event.Success {
			loginSuccess++
		} else {
			loginFailure++
		}
	}
	for _, session := range sessions {
		if session.RevokedAt != nil {
			revokedSessions++
			continue
		}
		activeSessions++
	}
	for _, event := range audits {
		switch event.Action {
		case "auth.refresh":
			refreshCount++
		case "auth.logout", "auth.logout_all":
			logoutCount++
		}
	}

	return &domainmonitoring.AuthStats{
		LoginSuccessCount:   loginSuccess,
		LoginFailureCount:   loginFailure,
		ActiveSessionCount:  activeSessions,
		RevokedSessionCount: revokedSessions,
		RefreshCount:        refreshCount,
		LogoutCount:         logoutCount,
		From:                from,
		To:                  to,
	}, nil
}

var _ domainmonitoring.AuthStatsRepository = (*memoryAuthStatsRepository)(nil)
