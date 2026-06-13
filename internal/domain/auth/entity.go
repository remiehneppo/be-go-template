// Package auth defines the Session, LoginHistory, and DeviceSession entities
// used by the authentication domain.
//
// Entities are plain structs without methods so they can be marshalled
// and persisted without coupling to infrastructure.

package auth

import (
	"regexp"
	"strings"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/user"
)

type Session struct {
	ID                    string
	UserID                string
	RefreshTokenHash      string
	RefreshTokenExpiresAt time.Time
	DeviceID              string
	DeviceName            string
	UserAgent             string
	IP                    string
	TokenFamilyID         string
	RevokedAt             *time.Time
	RevokedReason         string
	LastSeenAt            time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type LoginHistory struct {
	ID            string
	UserID        string
	Email         string
	Success       bool
	FailureReason string
	IP            string
	UserAgent     string
	DeviceID      string
	CreatedAt     time.Time
}

type DeviceSession struct {
	SessionID    string
	DeviceID     string
	DeviceName   string
	UserAgent    string
	IP           string
	LastSeenAt   time.Time
	CreatedAt    time.Time
	Current      bool
	RevokedAt    *time.Time
	RevokeReason string
}

type AuditLog struct {
	ID           string
	ActorUserID  string
	Action       string
	ResourceType string
	ResourceID   string
	IP           string
	UserAgent    string
	RequestID    string
	Metadata     map[string]string
	CreatedAt    time.Time
}

type ErrorEvent struct {
	RequestID string
	ErrorCode string
	Operation string
	Message   string
	Cause     string
	Stack     string
	Path      string
	Method    string
	Status    int
	UserID    string
	CreatedAt time.Time
}

type RequestMeta struct {
	RequestID  string
	IP         string
	UserAgent  string
	DeviceID   string
	DeviceName string
}

type RegisterInput struct {
	Email    string
	Password string
	Name     string
}

type LoginInput struct {
	Email    string
	Password string
}

type AuthResult struct {
	User                  user.User
	SessionID             string
	Session               DeviceSession
	AccessToken           string
	AccessTokenExpiresAt  time.Time
	RefreshToken          string
	RefreshTokenExpiresAt time.Time
}

func (s Session) IsActive(now time.Time) bool {
	return s.RevokedAt == nil && s.RefreshTokenExpiresAt.After(now)
}

func NormalizeDeviceID(deviceID string) string {
	deviceID = strings.TrimSpace(deviceID)
	if len(deviceID) > 64 {
		return ""
	}
	if !uuidV4Pattern.MatchString(deviceID) {
		return ""
	}
	return strings.ToLower(deviceID)
}

var uuidV4Pattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
