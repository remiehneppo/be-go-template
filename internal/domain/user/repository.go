package user

import (
	"context"
	"time"
)

type Repository interface {
	Create(ctx context.Context, user User) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	EnsureRole(ctx context.Context, userID string, role Role, updatedAt time.Time) error
	UpdateLastLogin(ctx context.Context, userID string, at time.Time) error
}
