package seed

import (
	"context"
	"testing"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/database"
)

func TestAdminSeederCreatesAdmin(t *testing.T) {
	repo := &fakeUserRepository{findErr: database.ErrNotFound}
	seeder := NewAdminSeeder(AdminSeederDependencies{
		Users:     repo,
		Passwords: fakePasswordHasher{},
		Now:       func() time.Time { return time.Unix(100, 0).UTC() },
		NewID:     func() (string, error) { return "admin-id", nil },
	})

	result, err := seeder.SeedAdmin(context.Background(), AdminInput{Email: " ADMIN@example.com ", Password: "secret", Name: "Root"})
	if err != nil {
		t.Fatalf("SeedAdmin() error = %v", err)
	}
	if !result.Created || result.Email != "admin@example.com" {
		t.Fatalf("result = %+v", result)
	}
	if repo.created.ID != "admin-id" || !repo.created.HasRole(user.RoleAdmin) || repo.created.PasswordHash != "hashed-secret" {
		t.Fatalf("created = %+v", repo.created)
	}
}

func TestAdminSeederAddsAdminRoleToExistingUser(t *testing.T) {
	repo := &fakeUserRepository{found: &user.User{ID: "u1", Email: "user@example.com", Roles: []user.Role{user.RoleUser}}}
	seeder := NewAdminSeeder(AdminSeederDependencies{Users: repo, Passwords: fakePasswordHasher{}})

	result, err := seeder.SeedAdmin(context.Background(), AdminInput{Email: "user@example.com", Password: "secret"})
	if err != nil {
		t.Fatalf("SeedAdmin() error = %v", err)
	}
	if !result.Updated || repo.ensuredRole != user.RoleAdmin || repo.ensuredUserID != "u1" {
		t.Fatalf("result = %+v ensured = %q %q", result, repo.ensuredUserID, repo.ensuredRole)
	}
}

func TestAdminSeederIsIdempotentForExistingAdmin(t *testing.T) {
	repo := &fakeUserRepository{found: &user.User{ID: "admin", Email: "admin@example.com", Roles: []user.Role{user.RoleAdmin}}}
	seeder := NewAdminSeeder(AdminSeederDependencies{Users: repo, Passwords: fakePasswordHasher{}})

	result, err := seeder.SeedAdmin(context.Background(), AdminInput{Email: "admin@example.com", Password: "secret"})
	if err != nil {
		t.Fatalf("SeedAdmin() error = %v", err)
	}
	if result.Created || result.Updated || repo.ensureCalls != 0 || repo.createCalls != 0 {
		t.Fatalf("result = %+v repo = %+v", result, repo)
	}
}

type fakeUserRepository struct {
	found         *user.User
	findErr       error
	created       user.User
	createCalls   int
	ensureCalls   int
	ensuredUserID string
	ensuredRole   user.Role
}

func (r *fakeUserRepository) Create(ctx context.Context, usr user.User) error {
	r.createCalls++
	r.created = usr
	return nil
}

func (r *fakeUserRepository) FindByID(ctx context.Context, id string) (*user.User, error) {
	return r.found, r.findErr
}

func (r *fakeUserRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	return r.found, r.findErr
}

func (r *fakeUserRepository) EnsureRole(ctx context.Context, userID string, role user.Role, updatedAt time.Time) error {
	r.ensureCalls++
	r.ensuredUserID = userID
	r.ensuredRole = role
	return nil
}

func (r *fakeUserRepository) UpdateLastLogin(ctx context.Context, userID string, at time.Time) error {
	return nil
}

func (r *fakeUserRepository) RecordLoginFailure(ctx context.Context, userID string, email string, failedAttempts int, lockedUntil *time.Time, updatedAt time.Time) error {
	return nil
}

func (r *fakeUserRepository) ResetLoginFailures(ctx context.Context, userID string, email string, updatedAt time.Time) error {
	return nil
}

type fakePasswordHasher struct{}

func (fakePasswordHasher) Hash(password string) (string, error) {
	return "hashed-" + password, nil
}

func (fakePasswordHasher) Compare(hash string, password string) error {
	return nil
}
