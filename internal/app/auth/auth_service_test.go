package auth

import (
	"context"
	"testing"
	"time"

	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
)

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
	revokedUserID string
	revokedReason string
}

func (r *fakeSessionRepository) Create(ctx context.Context, session domainauth.Session) error {
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

type fakeLoginHistoryRepository struct{}

func (r fakeLoginHistoryRepository) Append(ctx context.Context, event domainauth.LoginHistory) error {
	return nil
}

func (r fakeLoginHistoryRepository) ListByUserID(ctx context.Context, userID string, pagination common.Pagination) ([]domainauth.LoginHistory, error) {
	return nil, nil
}
