package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/remihneppo/be-go-template/internal/config"
	"github.com/remihneppo/be-go-template/internal/platform/migration"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func TestReadPreferenceParsesKnownValues(t *testing.T) {
	tests := map[string]*readpref.ReadPref{
		"primary":            readpref.Primary(),
		"primaryPreferred":   readpref.PrimaryPreferred(),
		"secondary":          readpref.Secondary(),
		"secondaryPreferred": readpref.SecondaryPreferred(),
		"nearest":            readpref.Nearest(),
		"unknown":            readpref.Primary(),
	}
	for input, want := range tests {
		got := readPreference(input)
		if got.Mode() != want.Mode() {
			t.Fatalf("readPreference(%q) = %s, want %s", input, got.Mode(), want.Mode())
		}
	}
}

func TestMigrationsReturnsBootstrapIndexes(t *testing.T) {
	t.Parallel()
	items := migrations(&mongo.Database{})
	if len(items) != 1 {
		t.Fatalf("migrations len = %d", len(items))
	}
	if items[0].Version != "202606110001" || items[0].Name != "bootstrap_indexes" {
		t.Fatalf("migration = %+v", items[0])
	}
}

func TestRunMigrationsPrintsAppliedVersions(t *testing.T) {
	t.Parallel()
	cfg := config.Config{}
	cfg.Mongo.ConnectTimeout = time.Second

	var runCalls int
	var received []migration.Migration
	var out bytes.Buffer
	err := runMigrations(context.Background(), cfg, &out, func(ctx context.Context, cfg config.Config) (*migrationRunnerHandle, error) {
		return &migrationRunnerHandle{
			Runner: fakeMigrationRunner{
				run: func(ctx context.Context, items []migration.Migration) ([]migration.AppliedMigration, error) {
					runCalls++
					received = append([]migration.Migration(nil), items...)
					return []migration.AppliedMigration{{Version: items[0].Version, Name: items[0].Name}}, nil
				},
			},
			Database: func() *mongo.Database {
				return &mongo.Database{}
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("runMigrations() error = %v", err)
	}
	if runCalls != 1 {
		t.Fatalf("runCalls = %d", runCalls)
	}
	if len(received) != 1 || received[0].Version != "202606110001" || received[0].Name != "bootstrap_indexes" {
		t.Fatalf("received = %+v", received)
	}
	if got, want := out.String(), "migrations applied: 1\n- 202606110001 bootstrap_indexes\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

type fakeMigrationRunner struct {
	run func(ctx context.Context, items []migration.Migration) ([]migration.AppliedMigration, error)
}

func (r fakeMigrationRunner) Run(ctx context.Context, items []migration.Migration) ([]migration.AppliedMigration, error) {
	return r.run(ctx, items)
}
