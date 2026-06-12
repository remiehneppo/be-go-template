package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/remihneppo/be-go-template/internal/app/seed"
	"github.com/remihneppo/be-go-template/internal/config"
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

func TestRunSeedPrintsStatusForAdminResult(t *testing.T) {
	t.Parallel()
	cfg := config.Config{}
	cfg.Mongo.ConnectTimeout = time.Second

	tests := []struct {
		name   string
		result *seed.AdminResult
		want   string
	}{
		{name: "created", result: &seed.AdminResult{Email: "admin@example.com", Created: true}, want: "admin user created: admin@example.com\n"},
		{name: "updated", result: &seed.AdminResult{Email: "admin@example.com", Updated: true}, want: "admin role granted: admin@example.com\n"},
		{name: "existing", result: &seed.AdminResult{Email: "admin@example.com"}, want: "admin user already present: admin@example.com\n"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			called := false
			err := runSeed(context.Background(), cfg, seed.AdminInput{Email: "admin@example.com", Password: "secret"}, &out, func(ctx context.Context, cfg config.Config) (*adminSeederHandle, error) {
				called = true
				return &adminSeederHandle{
					Seeder: fakeAdminSeeder{result: tt.result},
				}, nil
			})
			if err != nil {
				t.Fatalf("runSeed() error = %v", err)
			}
			if !called {
				t.Fatal("builder was not called")
			}
			if got := out.String(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

type fakeAdminSeeder struct {
	result *seed.AdminResult
}

func (s fakeAdminSeeder) SeedAdmin(ctx context.Context, input seed.AdminInput) (*seed.AdminResult, error) {
	return s.result, nil
}
