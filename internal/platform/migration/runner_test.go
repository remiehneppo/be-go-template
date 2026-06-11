package migration

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunnerAppliesPendingMigrationsOnceInVersionOrder(t *testing.T) {
	store := &fakeStore{applied: map[string]bool{"001": true}}
	runner := NewRunner(store)
	runner.now = func() time.Time { return time.Unix(100, 0).UTC() }
	var order []string

	applied, err := runner.Run(context.Background(), []Migration{
		{Version: "002", Name: "second", Apply: func(ctx context.Context) error {
			order = append(order, "002")
			return nil
		}},
		{Version: "001", Name: "first", Apply: func(ctx context.Context) error {
			order = append(order, "001")
			return nil
		}},
		{Version: "003", Name: "third", Apply: func(ctx context.Context) error {
			order = append(order, "003")
			return nil
		}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(applied) != 2 || applied[0].Version != "002" || applied[1].Version != "003" {
		t.Fatalf("applied = %+v", applied)
	}
	if len(order) != 2 || order[0] != "002" || order[1] != "003" {
		t.Fatalf("order = %v", order)
	}
}

func TestRunnerDoesNotRecordFailedMigration(t *testing.T) {
	store := &fakeStore{applied: map[string]bool{}}
	runner := NewRunner(store)

	_, err := runner.Run(context.Background(), []Migration{{
		Version: "001",
		Apply:   func(ctx context.Context) error { return errors.New("boom") },
	}})
	if err == nil {
		t.Fatal("Run() error = nil")
	}
	if store.applied["001"] {
		t.Fatal("failed migration was recorded")
	}
}

type fakeStore struct {
	applied map[string]bool
}

func (s *fakeStore) Ensure(ctx context.Context) error {
	return nil
}

func (s *fakeStore) Has(ctx context.Context, version string) (bool, error) {
	return s.applied[version], nil
}

func (s *fakeStore) Record(ctx context.Context, migration AppliedMigration) error {
	s.applied[migration.Version] = true
	return nil
}
