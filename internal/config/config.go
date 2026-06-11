package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	App  AppConfig
	HTTP HTTPConfig
	Log  LogConfig
}

type AppConfig struct {
	Name string
	Env  string
}

type HTTPConfig struct {
	Addr             string
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	IdleTimeout      time.Duration
	BodyLimitBytes   int64
	RouteTimeout     time.Duration
	CORSAllowOrigins []string
}

type LogConfig struct {
	Level      string
	Format     string
	FilePath   string
	ToTerminal bool
	ToFile     bool
}

func Load() (Config, error) {
	cfg := Config{
		App: AppConfig{
			Name: getString("APP_NAME", "be-go-template"),
			Env:  getString("APP_ENV", "local"),
		},
		HTTP: HTTPConfig{
			Addr:             getString("HTTP_ADDR", ":8080"),
			ReadTimeout:      getDuration("HTTP_READ_TIMEOUT", 5*time.Second),
			WriteTimeout:     getDuration("HTTP_WRITE_TIMEOUT", 10*time.Second),
			IdleTimeout:      getDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
			BodyLimitBytes:   getInt64("HTTP_BODY_LIMIT_BYTES", 1<<20),
			RouteTimeout:     getDuration("ROUTE_TIMEOUT_DEFAULT", 5*time.Second),
			CORSAllowOrigins: getCSV("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000", "http://localhost:5173"}),
		},
		Log: LogConfig{
			Level:      strings.ToLower(getString("LOG_LEVEL", "info")),
			Format:     strings.ToLower(getString("LOG_FORMAT", "json")),
			FilePath:   getString("LOG_FILE_PATH", "logs/app.log"),
			ToTerminal: getBool("LOG_TO_TERMINAL", true),
			ToFile:     getBool("LOG_TO_FILE", false),
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (cfg Config) Validate() error {
	if cfg.App.Name == "" {
		return fmt.Errorf("APP_NAME must not be empty")
	}
	if cfg.App.Env == "production" && len(cfg.HTTP.CORSAllowOrigins) == 0 {
		return fmt.Errorf("CORS_ALLOWED_ORIGINS is required in production")
	}
	if cfg.HTTP.BodyLimitBytes <= 0 {
		return fmt.Errorf("HTTP_BODY_LIMIT_BYTES must be positive")
	}
	if cfg.HTTP.RouteTimeout <= 0 {
		return fmt.Errorf("ROUTE_TIMEOUT_DEFAULT must be positive")
	}
	switch cfg.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error")
	}
	switch cfg.Log.Format {
	case "json", "text":
	default:
		return fmt.Errorf("LOG_FORMAT must be json or text")
	}
	if !cfg.Log.ToTerminal && !cfg.Log.ToFile {
		return fmt.Errorf("at least one log output must be enabled")
	}
	return nil
}

func getString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getCSV(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
