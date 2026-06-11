package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	App       AppConfig
	HTTP      HTTPConfig
	Log       LogConfig
	JWT       JWTConfig
	Mongo     MongoConfig
	Redis     RedisConfig
	RateLimit RateLimitConfig
	Auth      AuthConfig
	Metrics   MetricsConfig
	Readiness ReadinessConfig
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

type JWTConfig struct {
	AccessCurrentKey  string
	AccessPreviousKey string
	PreviousNotAfter  time.Time
	AccessTTL         time.Duration
	RefreshTTL        time.Duration
}

type MongoConfig struct {
	URI            string
	Database       string
	MaxPoolSize    uint64
	MinPoolSize    uint64
	ConnectTimeout time.Duration
	ReadPreference string
}

type RedisConfig struct {
	Addr          string
	Password      string
	DB            int
	LockPrefix    string
	TLSEnabled    bool
	TLSServerName string
}

type RateLimitConfig struct {
	AuthEnabled       bool
	LoginPerMinute    int64
	RefreshPerMinute  int64
	RegisterPerMinute int64
	Fallback          string
}

type AuthConfig struct {
	LockoutMaxFailures int
	LockoutDuration    time.Duration
}

type MetricsConfig struct {
	Enabled bool
	Path    string
}

type ReadinessConfig struct {
	Timeout                time.Duration
	RequiresRedis          bool
	MongoDegradedThreshold time.Duration
	RedisDegradedThreshold time.Duration
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
		JWT: JWTConfig{
			AccessCurrentKey:  getString("JWT_ACCESS_CURRENT_KEY", "local/"+base64Secret("local-access-secret-change-me")),
			AccessPreviousKey: getString("JWT_ACCESS_PREVIOUS_KEY", ""),
			PreviousNotAfter:  getTime("JWT_ACCESS_PREVIOUS_NOT_AFTER", time.Time{}),
			AccessTTL:         getDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL:        getDuration("JWT_REFRESH_TTL", 30*24*time.Hour),
		},
		Mongo: MongoConfig{
			URI:            getString("MONGO_URI", "mongodb://localhost:27017"),
			Database:       getString("MONGO_DATABASE", "be_go_template"),
			MaxPoolSize:    uint64(getInt64("MONGO_MAX_POOL_SIZE", 100)),
			MinPoolSize:    uint64(getInt64("MONGO_MIN_POOL_SIZE", 0)),
			ConnectTimeout: getDuration("MONGO_CONNECT_TIMEOUT", 10*time.Second),
			ReadPreference: getString("MONGO_READ_PREFERENCE", "primary"),
		},
		Redis: RedisConfig{
			Addr:          getString("REDIS_ADDR", "localhost:6379"),
			Password:      getString("REDIS_PASSWORD", ""),
			DB:            int(getInt64("REDIS_DB", 0)),
			LockPrefix:    getString("REDIS_LOCK_PREFIX", "lock:"),
			TLSEnabled:    getBool("REDIS_TLS_ENABLED", false),
			TLSServerName: getString("REDIS_TLS_SERVER_NAME", ""),
		},
		RateLimit: RateLimitConfig{
			AuthEnabled:       getBool("AUTH_RATE_LIMIT_ENABLED", true),
			LoginPerMinute:    getInt64("AUTH_RATE_LIMIT_LOGIN_PER_MINUTE", 10),
			RefreshPerMinute:  getInt64("AUTH_RATE_LIMIT_REFRESH_PER_MINUTE", 30),
			RegisterPerMinute: getInt64("AUTH_RATE_LIMIT_REGISTER_PER_MINUTE", 5),
			Fallback:          strings.ToLower(getString("RATE_LIMIT_FALLBACK", rateLimitFallbackDefault(getString("APP_ENV", "local")))),
		},
		Auth: AuthConfig{
			LockoutMaxFailures: int(getInt64("AUTH_LOCKOUT_MAX_FAILURES", 5)),
			LockoutDuration:    getDuration("AUTH_LOCKOUT_DURATION", 15*time.Minute),
		},
		Metrics: MetricsConfig{
			Enabled: getBool("METRICS_ENABLED", true),
			Path:    getString("METRICS_PATH", "/metrics"),
		},
		Readiness: ReadinessConfig{
			Timeout:                getDuration("READY_TIMEOUT", 2*time.Second),
			RequiresRedis:          getBool("READY_REQUIRES_REDIS", getString("APP_ENV", "local") == "production"),
			MongoDegradedThreshold: getDuration("MONGO_DEGRADED_THRESHOLD", 500*time.Millisecond),
			RedisDegradedThreshold: getDuration("REDIS_DEGRADED_THRESHOLD", 200*time.Millisecond),
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
	if cfg.JWT.AccessCurrentKey == "" {
		return fmt.Errorf("JWT_ACCESS_CURRENT_KEY must not be empty")
	}
	if cfg.JWT.AccessTTL <= 0 {
		return fmt.Errorf("JWT_ACCESS_TTL must be positive")
	}
	if cfg.JWT.RefreshTTL <= 0 {
		return fmt.Errorf("JWT_REFRESH_TTL must be positive")
	}
	if cfg.Mongo.URI == "" {
		return fmt.Errorf("MONGO_URI must not be empty")
	}
	if cfg.Mongo.Database == "" {
		return fmt.Errorf("MONGO_DATABASE must not be empty")
	}
	if cfg.Mongo.ConnectTimeout <= 0 {
		return fmt.Errorf("MONGO_CONNECT_TIMEOUT must be positive")
	}
	if cfg.Redis.Addr == "" {
		return fmt.Errorf("REDIS_ADDR must not be empty")
	}
	if cfg.Redis.DB < 0 {
		return fmt.Errorf("REDIS_DB must not be negative")
	}
	if cfg.RateLimit.LoginPerMinute < 0 || cfg.RateLimit.RefreshPerMinute < 0 || cfg.RateLimit.RegisterPerMinute < 0 {
		return fmt.Errorf("auth rate limit values must not be negative")
	}
	if cfg.Auth.LockoutMaxFailures < 0 {
		return fmt.Errorf("AUTH_LOCKOUT_MAX_FAILURES must not be negative")
	}
	if cfg.Auth.LockoutMaxFailures > 0 && cfg.Auth.LockoutDuration <= 0 {
		return fmt.Errorf("AUTH_LOCKOUT_DURATION must be positive when lockout is enabled")
	}
	switch cfg.RateLimit.Fallback {
	case "allow", "block":
	default:
		return fmt.Errorf("RATE_LIMIT_FALLBACK must be allow or block")
	}
	if cfg.Metrics.Enabled && (cfg.Metrics.Path == "" || !strings.HasPrefix(cfg.Metrics.Path, "/")) {
		return fmt.Errorf("METRICS_PATH must start with /")
	}
	if cfg.Readiness.Timeout <= 0 {
		return fmt.Errorf("READY_TIMEOUT must be positive")
	}
	if cfg.Readiness.MongoDegradedThreshold <= 0 {
		return fmt.Errorf("MONGO_DEGRADED_THRESHOLD must be positive")
	}
	if cfg.Readiness.RedisDegradedThreshold <= 0 {
		return fmt.Errorf("REDIS_DEGRADED_THRESHOLD must be positive")
	}
	return nil
}

func rateLimitFallbackDefault(env string) string {
	if env == "production" {
		return "block"
	}
	return "allow"
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

func getTime(key string, fallback time.Time) time.Time {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return fallback
	}
	return parsed
}

func base64Secret(secret string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(secret))
}
