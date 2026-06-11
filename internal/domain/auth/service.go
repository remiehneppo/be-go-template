package auth

import (
	"context"

	"github.com/remihneppo/be-go-template/internal/domain/common"
)

type Service interface {
	Register(ctx context.Context, input RegisterInput) (*AuthResult, error)
	Login(ctx context.Context, input LoginInput, meta RequestMeta) (*AuthResult, error)
	Refresh(ctx context.Context, refreshToken string, meta RequestMeta) (*AuthResult, error)
	Logout(ctx context.Context, accessToken string, sessionID string) error
	LogoutAll(ctx context.Context, userID string) error
	ListDevices(ctx context.Context, userID string) ([]DeviceSession, error)
	ListLoginHistory(ctx context.Context, userID string, pagination common.Pagination) ([]LoginHistory, error)
}
