package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("APP_NAME", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("LOG_TO_TERMINAL", "")
	t.Setenv("LOG_TO_FILE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.App.Name != "be-go-template" {
		t.Fatalf("App.Name = %q", cfg.App.Name)
	}
	if cfg.HTTP.BodyLimitBytes != 1<<20 {
		t.Fatalf("BodyLimitBytes = %d", cfg.HTTP.BodyLimitBytes)
	}
	if len(cfg.HTTP.CORSAllowOrigins) != 2 {
		t.Fatalf("CORSAllowOrigins len = %d", len(cfg.HTTP.CORSAllowOrigins))
	}
	if !cfg.RateLimit.AuthEnabled || cfg.RateLimit.Fallback != "allow" {
		t.Fatalf("RateLimit = %+v", cfg.RateLimit)
	}
	if cfg.Auth.LockoutMaxFailures != 5 || cfg.Auth.LockoutDuration <= 0 {
		t.Fatalf("Auth = %+v", cfg.Auth)
	}
	if !cfg.Metrics.Enabled || cfg.Metrics.Path != "/metrics" {
		t.Fatalf("Metrics = %+v", cfg.Metrics)
	}
	if cfg.Readiness.Timeout <= 0 || cfg.Readiness.RequiresRedis {
		t.Fatalf("Readiness = %+v", cfg.Readiness)
	}
}

func TestValidateRequiresLogOutput(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.Log.ToTerminal = false
	cfg.Log.ToFile = false

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil")
	}
}

func TestRateLimitFallbackDefaultsToBlockInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("RATE_LIMIT_FALLBACK", "")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.RateLimit.Fallback != "block" {
		t.Fatalf("Fallback = %q", cfg.RateLimit.Fallback)
	}
	if !cfg.Readiness.RequiresRedis {
		t.Fatal("Readiness.RequiresRedis = false")
	}
}
