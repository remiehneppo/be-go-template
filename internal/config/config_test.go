package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("APP_NAME", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("LOG_TO_CONSOLE", "")
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
	if !cfg.Errors.IncludeStack {
		t.Fatal("Errors.IncludeStack = false")
	}
	if !cfg.Outbox.Enabled || cfg.Outbox.DrainInterval <= 0 || cfg.Outbox.BatchSize != 10 || cfg.Outbox.DefaultMaxRetries != 10 || cfg.Outbox.RetryDelay <= 0 {
		t.Fatalf("Outbox = %+v", cfg.Outbox)
	}
	if cfg.Log.MaxSizeMB != 100 || cfg.Log.MaxBackups != 10 || cfg.Log.MaxAgeDays != 30 || !cfg.Log.Compress {
		t.Fatalf("Log rotation defaults = %+v", cfg.Log)
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

func TestLoadSupportsConsoleAlias(t *testing.T) {
	t.Setenv("LOG_TO_CONSOLE", "true")
	t.Setenv("LOG_TO_TERMINAL", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Log.ToTerminal {
		t.Fatal("LOG_TO_CONSOLE should override LOG_TO_TERMINAL")
	}
}

func TestValidateRequiresRotationConfigWhenFileLoggingEnabled(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.Log.ToFile = true
	cfg.Log.FilePath = ""

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

func TestLoadSupportsOutboxConfig(t *testing.T) {
	t.Setenv("OUTBOX_ENABLED", "false")
	t.Setenv("OUTBOX_DRAIN_INTERVAL", "2s")
	t.Setenv("OUTBOX_BATCH_SIZE", "25")
	t.Setenv("OUTBOX_MAX_RETRIES", "7")
	t.Setenv("OUTBOX_RETRY_DELAY", "3m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Outbox.Enabled {
		t.Fatal("Outbox.Enabled = true")
	}
	if cfg.Outbox.DrainInterval != 2*time.Second || cfg.Outbox.BatchSize != 25 || cfg.Outbox.DefaultMaxRetries != 7 || cfg.Outbox.RetryDelay != 3*time.Minute {
		t.Fatalf("Outbox = %+v", cfg.Outbox)
	}
}

func TestLoadSupportsErrorStackConfig(t *testing.T) {
	t.Setenv("ERROR_INCLUDE_STACK", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Errors.IncludeStack {
		t.Fatal("Errors.IncludeStack = true")
	}
}
