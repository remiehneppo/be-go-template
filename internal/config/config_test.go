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
