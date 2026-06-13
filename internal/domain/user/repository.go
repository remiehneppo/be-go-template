// UserRepository defines the persistence contract for User entities.
package user

import (
	"context"
	"time"
)

// Repository abstracts user persistence operations. Implementations must be
// safe for concurrent use by multiple goroutines.
type Repository interface {
	// Create persists a new user and returns the generated ID.
	Create(ctx context.Context, user User) error
	// FindByID retrieves a user by ID.
	FindByID(ctx context.Context, id string) (*User, error)
	// FindByEmail retrieves a user by email.
	FindByEmail(ctx context.Context, email string) (*User, error)
	// EnsureRole grants a role to a user if they don't already have it.
	EnsureRole(ctx context.Context, userID string, role Role, updatedAt time.Time) error
	// UpdateLastLogin updates the last login timestamp.
	UpdateLastLogin(ctx context.Context, userID string, at time.Time) error
	// RecordLoginFailure records a failed login attempt for lockout tracking.
	RecordLoginFailure(ctx context.Context, userID string, email string, failedAttempts int, lockedUntil *time.Time, updatedAt time.Time) error
	// ResetLoginFailures resets failed login attempts after a successful login.
	ResetLoginFailures(ctx context.Context, userID string, email string, updatedAt time.Time) error
}
