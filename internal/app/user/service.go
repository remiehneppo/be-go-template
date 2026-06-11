package user

import (
	"context"
	"net/http"
	"strings"

	domainuser "github.com/remihneppo/be-go-template/internal/domain/user"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
)

type Service struct {
	users domainuser.Repository
}

func NewService(users domainuser.Repository) *Service {
	return &Service{users: users}
}

func (s *Service) GetMe(ctx context.Context, userID string) (*domainuser.User, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, apperrors.New(apperrors.CodeUnauthorized, "Unauthorized", http.StatusUnauthorized)
	}
	if s.users == nil {
		return nil, apperrors.New(apperrors.CodeInternal, "User service is not configured", http.StatusInternalServerError)
	}
	return s.users.FindByID(ctx, userID)
}

var _ domainuser.Service = (*Service)(nil)
