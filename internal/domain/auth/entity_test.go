package auth

import (
	"testing"
	"time"
)

func TestNormalizeDeviceIDRequiresUUIDV4(t *testing.T) {
	valid := "550e8400-e29b-41d4-a716-446655440000"
	if got := NormalizeDeviceID(valid); got != valid {
		t.Fatalf("NormalizeDeviceID(valid) = %q", got)
	}
	if got := NormalizeDeviceID("550e8400-e29b-11d4-a716-446655440000"); got != "" {
		t.Fatalf("NormalizeDeviceID(v1) = %q", got)
	}
	if got := NormalizeDeviceID("x" + valid); got != "" {
		t.Fatalf("NormalizeDeviceID(invalid) = %q", got)
	}
	if got := NormalizeDeviceID(valid + valid); got != "" {
		t.Fatalf("NormalizeDeviceID(oversized) = %q", got)
	}
}

func TestSessionIsActive(t *testing.T) {
	now := time.Unix(10, 0)
	session := Session{RefreshTokenExpiresAt: now.Add(time.Minute)}
	if !session.IsActive(now) {
		t.Fatal("session should be active")
	}
	revokedAt := now
	session.RevokedAt = &revokedAt
	if session.IsActive(now) {
		t.Fatal("revoked session should be inactive")
	}
}
