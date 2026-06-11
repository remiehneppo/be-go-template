package auth

import (
	"context"
	"net/http"
	"time"

	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	"github.com/remihneppo/be-go-template/internal/domain/user"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
)

type ServiceDependencies struct {
	Users        user.Repository
	Sessions     domainauth.SessionRepository
	LoginHistory domainauth.LoginHistoryRepository
	Tokens       domainauth.TokenService
}

type Service struct {
	users        user.Repository
	sessions     domainauth.SessionRepository
	loginHistory domainauth.LoginHistoryRepository
	tokens       domainauth.TokenService
	now          func() time.Time
}

func NewService(deps ServiceDependencies) *Service {
	return &Service{
		users:        deps.Users,
		sessions:     deps.Sessions,
		loginHistory: deps.LoginHistory,
		tokens:       deps.Tokens,
		now:          func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Register(ctx context.Context, input domainauth.RegisterInput) (*domainauth.AuthResult, error) {
	return nil, notImplemented("AuthService.Register")
}

func (s *Service) Login(ctx context.Context, input domainauth.LoginInput, meta domainauth.RequestMeta) (*domainauth.AuthResult, error) {
	return nil, notImplemented("AuthService.Login")
}

func (s *Service) Refresh(ctx context.Context, refreshToken string, meta domainauth.RequestMeta) (*domainauth.AuthResult, error) {
	return nil, notImplemented("AuthService.Refresh")
}

func (s *Service) Logout(ctx context.Context, accessToken string, sessionID string) error {
	return notImplemented("AuthService.Logout")
}

func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	if s.sessions == nil {
		return notImplemented("AuthService.LogoutAll")
	}
	return s.sessions.RevokeAllByUserID(ctx, userID, "logout_all", s.now())
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

func notImplemented(op string) error {
	return apperrors.New(apperrors.CodeDependency, op+" is not implemented in this checkpoint", http.StatusServiceUnavailable)
}

var _ domainauth.Service = (*Service)(nil)
