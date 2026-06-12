package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Config struct {
	App        AppConfig
	HTTP       HTTPConfig
	Log        LogConfig
	JWT        JWTConfig
	Mongo      MongoConfig
	Redis      RedisConfig
	RateLimit  RateLimitConfig
	Auth       AuthConfig
	Monitoring MonitoringConfig
	Errors     ErrorConfig
	Outbox     OutboxConfig
	Metrics    MetricsConfig
	Readiness  ReadinessConfig
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
	ETagEnabled      bool
	CORSAllowOrigins []string
	CORSAllowMethods []string
	CORSAllowHeaders []string
}

type LogConfig struct {
	Level      string
	Format     string
	FilePath   string
	ToTerminal bool
	ToFile     bool
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
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
	MaxPoolSize    int64
	MinPoolSize    int64
	ConnectTimeout time.Duration
	ReadPreference string
}

type RedisConfig struct {
	Addr          string
	Password      string
	DB            int
	LockPrefix    string
	TLSEnabled    bool
	TLSCACert     string
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
	BcryptCost         int
}

type MonitoringConfig struct {
	Enabled                bool
	AdminRoles             []string
	MetricsCollectInterval time.Duration
}

type ErrorConfig struct {
	IncludeStack bool
}

type OutboxConfig struct {
	Enabled           bool
	DrainInterval     time.Duration
	BatchSize         int
	DefaultMaxRetries int
	RetryDelay        time.Duration
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
			ETagEnabled:      getBool("ETAG_ENABLED", true),
			CORSAllowOrigins: getCSV("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000", "http://localhost:5173", "http://127.0.0.1:3000", "http://127.0.0.1:5173"}),
			CORSAllowMethods: getCSV("CORS_ALLOWED_METHODS", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}),
			CORSAllowHeaders: getCSV("CORS_ALLOWED_HEADERS", []string{"Authorization", "Content-Type", "X-Request-ID", "X-Trace-ID", "X-Span-ID", "X-Device-ID", "X-Device-Name"}),
		},
		Log: LogConfig{
			Level:      strings.ToLower(getString("LOG_LEVEL", "info")),
			Format:     strings.ToLower(getString("LOG_FORMAT", "json")),
			FilePath:   getString("LOG_FILE_PATH", "logs/app.log"),
			ToTerminal: getBoolAny("LOG_TO_CONSOLE", "LOG_TO_TERMINAL", true),
			ToFile:     getBool("LOG_TO_FILE", false),
			MaxSizeMB:  int(getInt64("LOG_MAX_SIZE_MB", 100)),
			MaxBackups: int(getInt64("LOG_MAX_BACKUPS", 10)),
			MaxAgeDays: int(getInt64("LOG_MAX_AGE_DAYS", 30)),
			Compress:   getBool("LOG_COMPRESS", true),
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
			MaxPoolSize:    getInt64("MONGO_MAX_POOL_SIZE", 100),
			MinPoolSize:    getInt64("MONGO_MIN_POOL_SIZE", 0),
			ConnectTimeout: getDuration("MONGO_CONNECT_TIMEOUT", 10*time.Second),
			ReadPreference: getString("MONGO_READ_PREFERENCE", "primary"),
		},
		Redis: RedisConfig{
			Addr:          getString("REDIS_ADDR", "localhost:6379"),
			Password:      getString("REDIS_PASSWORD", ""),
			DB:            int(getInt64("REDIS_DB", 0)),
			LockPrefix:    getString("REDIS_LOCK_PREFIX", "lock:"),
			TLSEnabled:    getBool("REDIS_TLS_ENABLED", false),
			TLSCACert:     getString("REDIS_TLS_CA_CERT", ""),
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
			BcryptCost:         int(getInt64("BCRYPT_COST", int64(bcrypt.DefaultCost))),
		},
		Monitoring: MonitoringConfig{
			Enabled:                getBool("MONITORING_ENABLED", true),
			AdminRoles:             getCSV("MONITORING_ADMIN_ROLES", []string{"admin"}),
			MetricsCollectInterval: getDuration("METRICS_COLLECT_INTERVAL", 30*time.Second),
		},
		Errors: ErrorConfig{
			IncludeStack: getBool("ERROR_INCLUDE_STACK", getString("APP_ENV", "local") != "production"),
		},
		Outbox: OutboxConfig{
			Enabled:           getBool("OUTBOX_ENABLED", true),
			DrainInterval:     getDuration("OUTBOX_DRAIN_INTERVAL", 5*time.Second),
			BatchSize:         int(getInt64("OUTBOX_BATCH_SIZE", 10)),
			DefaultMaxRetries: int(getInt64("OUTBOX_MAX_RETRIES", 10)),
			RetryDelay:        getDuration("OUTBOX_RETRY_DELAY", time.Minute),
		},
		Metrics: MetricsConfig{
			Enabled: getBoolAny("PROMETHEUS_ENABLED", "METRICS_ENABLED", true),
			Path:    getString("PROMETHEUS_PATH", getString("METRICS_PATH", "/metrics")),
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
	if cfg.App.Env == "production" {
		for _, origin := range cfg.HTTP.CORSAllowOrigins {
			if strings.Contains(origin, "*") {
				return fmt.Errorf("CORS_ALLOWED_ORIGINS must not contain wildcard in production")
			}
		}
	}
	if cfg.HTTP.BodyLimitBytes <= 0 {
		return fmt.Errorf("HTTP_BODY_LIMIT_BYTES must be positive")
	}
	if cfg.HTTP.ReadTimeout <= 0 {
		return fmt.Errorf("HTTP_READ_TIMEOUT must be positive")
	}
	if cfg.HTTP.WriteTimeout <= 0 {
		return fmt.Errorf("HTTP_WRITE_TIMEOUT must be positive")
	}
	if cfg.HTTP.IdleTimeout <= 0 {
		return fmt.Errorf("HTTP_IDLE_TIMEOUT must be positive")
	}
	if cfg.HTTP.RouteTimeout <= 0 {
		return fmt.Errorf("ROUTE_TIMEOUT_DEFAULT must be positive")
	}
	if cfg.Mongo.MaxPoolSize <= 0 {
		return fmt.Errorf("MONGO_MAX_POOL_SIZE must be positive")
	}
	if cfg.Mongo.MinPoolSize < 0 {
		return fmt.Errorf("MONGO_MIN_POOL_SIZE must not be negative")
	}
	if cfg.Mongo.MinPoolSize > cfg.Mongo.MaxPoolSize {
		return fmt.Errorf("MONGO_MIN_POOL_SIZE must not exceed MONGO_MAX_POOL_SIZE")
	}
	switch cfg.Mongo.ReadPreference {
	case "primary", "primaryPreferred", "secondary", "secondaryPreferred", "nearest":
	default:
		return fmt.Errorf("MONGO_READ_PREFERENCE must be one of primary, primaryPreferred, secondary, secondaryPreferred, nearest")
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
	if cfg.Log.ToFile {
		if cfg.Log.FilePath == "" {
			return fmt.Errorf("LOG_FILE_PATH must not be empty when file logging is enabled")
		}
		if cfg.Log.MaxSizeMB <= 0 {
			return fmt.Errorf("LOG_MAX_SIZE_MB must be positive")
		}
		if cfg.Log.MaxBackups < 0 {
			return fmt.Errorf("LOG_MAX_BACKUPS must not be negative")
		}
		if cfg.Log.MaxAgeDays < 0 {
			return fmt.Errorf("LOG_MAX_AGE_DAYS must not be negative")
		}
	}
	if cfg.JWT.AccessCurrentKey == "" {
		return fmt.Errorf("JWT_ACCESS_CURRENT_KEY must not be empty")
	}
	if err := validateJWTKey(cfg.JWT.AccessCurrentKey, "JWT_ACCESS_CURRENT_KEY"); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.JWT.AccessPreviousKey) != "" {
		if err := validateJWTKey(cfg.JWT.AccessPreviousKey, "JWT_ACCESS_PREVIOUS_KEY"); err != nil {
			return err
		}
		if cfg.JWT.PreviousNotAfter.IsZero() {
			return fmt.Errorf("JWT_ACCESS_PREVIOUS_NOT_AFTER must be set when JWT_ACCESS_PREVIOUS_KEY is configured")
		}
		if !cfg.JWT.PreviousNotAfter.After(time.Now().UTC()) {
			return fmt.Errorf("JWT_ACCESS_PREVIOUS_NOT_AFTER must be in the future when JWT_ACCESS_PREVIOUS_KEY is configured")
		}
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
	if cfg.Auth.BcryptCost < bcrypt.MinCost || cfg.Auth.BcryptCost > bcrypt.MaxCost {
		return fmt.Errorf("BCRYPT_COST must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
	}
	if len(cfg.Monitoring.AdminRoles) == 0 {
		return fmt.Errorf("MONITORING_ADMIN_ROLES must not be empty")
	}
	if cfg.Monitoring.MetricsCollectInterval <= 0 {
		return fmt.Errorf("METRICS_COLLECT_INTERVAL must be positive")
	}
	if cfg.Outbox.DrainInterval <= 0 {
		return fmt.Errorf("OUTBOX_DRAIN_INTERVAL must be positive")
	}
	if cfg.Outbox.BatchSize <= 0 {
		return fmt.Errorf("OUTBOX_BATCH_SIZE must be positive")
	}
	if cfg.Outbox.DefaultMaxRetries <= 0 {
		return fmt.Errorf("OUTBOX_MAX_RETRIES must be positive")
	}
	if cfg.Outbox.RetryDelay <= 0 {
		return fmt.Errorf("OUTBOX_RETRY_DELAY must be positive")
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

func validateJWTKey(value string, field string) error {
	parts := strings.SplitN(strings.TrimSpace(value), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("%s must use <key-id>/<base64-secret> format", field)
	}
	secret, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("%s must contain valid base64 secret", field)
	}
	if len(secret) < 16 {
		return fmt.Errorf("%s secret must be at least 16 bytes", field)
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

func getBoolAny(key string, alias string, fallback bool) bool {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
		}
	}
	if alias != "" {
		if value := strings.TrimSpace(os.Getenv(alias)); value != "" {
			parsed, err := strconv.ParseBool(value)
			if err == nil {
				return parsed
			}
		}
	}
	return fallback
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
