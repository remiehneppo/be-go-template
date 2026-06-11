package user

import (
	"strings"
	"time"
)

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

type User struct {
	ID                  string
	Email               string
	PasswordHash        string
	Name                string
	Roles               []Role
	Status              Status
	FailedLoginAttempts int
	LockedUntil         *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
	LastLoginAt         *time.Time
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func New(email, passwordHash, name string, now time.Time) User {
	return User{
		Email:        NormalizeEmail(email),
		PasswordHash: passwordHash,
		Name:         strings.TrimSpace(name),
		Roles:        []Role{RoleUser},
		Status:       StatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func (u User) HasRole(role Role) bool {
	for _, current := range u.Roles {
		if current == role {
			return true
		}
	}
	return false
}

func (u User) IsLocked(now time.Time) bool {
	return u.LockedUntil != nil && u.LockedUntil.After(now)
}
