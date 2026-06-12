package database

import (
	"testing"
	"time"
)

func TestReadOptionsValidateAllowsZeroValue(t *testing.T) {
	if err := (ReadOptions{}).Validate(); err != nil {
		t.Fatalf("ReadOptions{}.Validate() error = %v", err)
	}
}

func TestReadOptionsValidateRequiresCacheKeyForCacheOrLock(t *testing.T) {
	cases := []ReadOptions{
		{CacheTTL: time.Second},
		{LockOnMiss: true},
	}
	for _, opts := range cases {
		if err := opts.Validate(); err == nil {
			t.Fatalf("ReadOptions.Validate() = nil for %+v", opts)
		}
	}
}

func TestReadOptionsValidateRequiresPositiveTTLWhenCacheKeySet(t *testing.T) {
	if err := (ReadOptions{CacheKey: "k"}).Validate(); err == nil {
		t.Fatal("ReadOptions.Validate() error = nil")
	}
}

func TestWriteOptionsValidateAllowsZeroValue(t *testing.T) {
	if err := (WriteOptions{}).Validate(); err != nil {
		t.Fatalf("WriteOptions{}.Validate() error = %v", err)
	}
}

func TestWriteOptionsValidateRequiresLockKeyWhenStrictLockEnabled(t *testing.T) {
	if err := (WriteOptions{StrictLock: true}).Validate(); err == nil {
		t.Fatal("WriteOptions.Validate() error = nil")
	}
}
