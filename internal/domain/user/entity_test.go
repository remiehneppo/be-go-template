package user

import (
	"testing"
	"time"
)

func TestNewUserNormalizesEmailAndDefaultsRole(t *testing.T) {
	got := New(" USER@Example.COM ", "hash", " User ", time.Unix(1, 0))
	if got.Email != "user@example.com" {
		t.Fatalf("Email = %q", got.Email)
	}
	if !got.HasRole(RoleUser) {
		t.Fatal("default role user missing")
	}
	if got.Status != StatusActive {
		t.Fatalf("Status = %q", got.Status)
	}
}
