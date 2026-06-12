package database

import (
	"context"
	"testing"
	"time"
)

func TestMongoDatabaseValidatesReadOptionsBeforeQuery(t *testing.T) {
	db := &MongoDatabase{}

	err := db.FindOne(context.Background(), "users", nil, nil, ReadOptions{CacheTTL: time.Second})
	if err == nil {
		t.Fatal("FindOne() error = nil")
	}
	if got, want := err.Error(), "read cache options require CacheKey"; got != want {
		t.Fatalf("FindOne() error = %q, want %q", got, want)
	}
}

func TestMongoDatabaseValidatesWriteOptionsBeforeQuery(t *testing.T) {
	db := &MongoDatabase{}

	err := db.InsertOne(context.Background(), "users", nil, WriteOptions{StrictLock: true})
	if err == nil {
		t.Fatal("InsertOne() error = nil")
	}
	if got, want := err.Error(), "StrictLock requires LockKey"; got != want {
		t.Fatalf("InsertOne() error = %q, want %q", got, want)
	}
}
