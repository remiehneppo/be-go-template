package user

import "context"

type Service interface {
	GetMe(ctx context.Context, userID string) (*User, error)
}
