package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestBcryptHasherUsesConfiguredCost(t *testing.T) {
	hasher := BcryptHasher{Cost: bcrypt.MinCost}

	hash, err := hasher.Hash("password123")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("bcrypt.Cost() error = %v", err)
	}
	if cost != bcrypt.MinCost {
		t.Fatalf("hash cost = %d", cost)
	}
}
