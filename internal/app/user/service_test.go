package user

import (
	"context"
	"testing"
	"time"

	domainuser "github.com/remihneppo/be-go-template/internal/domain/user"
)

func TestServiceGetMeFindsUserByID(t *testing.T) {
	repo := &fakeUserRepository{found: &domainuser.User{ID: "u1", Email: "user@example.com"}}
	service := NewService(repo)

	got, err := service.GetMe(context.Background(), " u1 ")
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	if got.ID != "u1" || repo.findByID != "u1" {
		t.Fatalf("got = %+v findByID = %q", got, repo.findByID)
	}
}

func TestServiceGetMeRejectsMissingUserID(t *testing.T) {
	service := NewService(&fakeUserRepository{})
	if _, err := service.GetMe(context.Background(), " "); err == nil {
		t.Fatal("GetMe() error = nil")
	}
}

type fakeUserRepository struct {
	found    *domainuser.User
	findByID string
}

func (r *fakeUserRepository) Create(ctx context.Context, user domainuser.User) error {
	return nil
}

func (r *fakeUserRepository) FindByID(ctx context.Context, id string) (*domainuser.User, error) {
	r.findByID = id
	return r.found, nil
}

func (r *fakeUserRepository) FindByEmail(ctx context.Context, email string) (*domainuser.User, error) {
	return r.found, nil
}

func (r *fakeUserRepository) EnsureRole(ctx context.Context, userID string, role domainuser.Role, updatedAt time.Time) error {
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
