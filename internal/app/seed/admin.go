package seed

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	appauth "github.com/remihneppo/be-go-template/internal/app/auth"
	"github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/database"
)

type AdminInput struct {
	Email    string
	Password string
	Name     string
}

type AdminResult struct {
	UserID  string
	Email   string
	Created bool
	Updated bool
}

type AdminSeeder struct {
	users     user.Repository
	passwords appauth.PasswordHasher
	now       func() time.Time
	newID     func() (string, error)
}

type AdminSeederDependencies struct {
	Users     user.Repository
	Passwords appauth.PasswordHasher
	Now       func() time.Time
	NewID     func() (string, error)
}

func NewAdminSeeder(deps AdminSeederDependencies) *AdminSeeder {
	passwords := deps.Passwords
	if passwords == nil {
		passwords = appauth.BcryptHasher{}
	}
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	newID := deps.NewID
	if newID == nil {
		newID = randomID
	}
	return &AdminSeeder{users: deps.Users, passwords: passwords, now: now, newID: newID}
}

func (s *AdminSeeder) SeedAdmin(ctx context.Context, input AdminInput) (*AdminResult, error) {
	if s.users == nil {
		return nil, fmt.Errorf("user repository is required")
	}
	email := user.NormalizeEmail(input.Email)
	if email == "" {
		return nil, fmt.Errorf("admin email is required")
	}
	if strings.TrimSpace(input.Password) == "" {
		return nil, fmt.Errorf("admin password is required")
	}
	existing, err := s.users.FindByEmail(ctx, email)
	if err == nil && existing != nil {
		if existing.HasRole(user.RoleAdmin) {
			return &AdminResult{UserID: existing.ID, Email: existing.Email}, nil
		}
		now := s.now()
		if err := s.users.EnsureRole(ctx, existing.ID, user.RoleAdmin, now); err != nil {
			return nil, err
		}
		return &AdminResult{UserID: existing.ID, Email: existing.Email, Updated: true}, nil
	}
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, err
	}

	now := s.now()
	id, err := s.newID()
	if err != nil {
		return nil, err
	}
	passwordHash, err := s.passwords.Hash(input.Password)
	if err != nil {
		return nil, err
	}
	admin := user.New(email, passwordHash, input.Name, now)
	admin.ID = id
	admin.Roles = []user.Role{user.RoleAdmin}
	if admin.Name == "" {
		admin.Name = "Administrator"
	}
	if err := s.users.Create(ctx, admin); err != nil {
		return nil, err
	}
	return &AdminResult{UserID: admin.ID, Email: admin.Email, Created: true}, nil
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
