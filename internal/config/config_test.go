package config

import (
	"encoding/base64"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
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
	if cfg.HTTP.ReadTimeout != 5*time.Second || cfg.HTTP.WriteTimeout != 10*time.Second || cfg.HTTP.IdleTimeout != 60*time.Second || cfg.HTTP.RouteTimeout != 5*time.Second {
		t.Fatalf("HTTP timeouts = %+v", cfg.HTTP)
	}
	if !cfg.HTTP.ETagEnabled {
		t.Fatal("HTTP.ETagEnabled = false")
	}
	wantOrigins := map[string]bool{
		"http://localhost:3000": true,
		"http://localhost:5173": true,
		"http://127.0.0.1:3000": true,
		"http://127.0.0.1:5173": true,
	}
	if len(cfg.HTTP.CORSAllowOrigins) != len(wantOrigins) {
		t.Fatalf("CORSAllowOrigins len = %d", len(cfg.HTTP.CORSAllowOrigins))
	}
	for _, origin := range cfg.HTTP.CORSAllowOrigins {
		if !wantOrigins[origin] {
			t.Fatalf("unexpected CORS origin %q", origin)
		}
	}
	if got, want := cfg.HTTP.CORSAllowMethods, []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}; !equalStrings(got, want) {
		t.Fatalf("CORSAllowMethods = %+v, want %+v", got, want)
	}
	if got, want := cfg.HTTP.CORSAllowHeaders, []string{"Authorization", "Content-Type", "X-Request-ID", "X-Trace-ID", "X-Span-ID", "X-Device-ID", "X-Device-Name"}; !equalStrings(got, want) {
		t.Fatalf("CORSAllowHeaders = %+v, want %+v", got, want)
	}
	if !cfg.RateLimit.AuthEnabled || cfg.RateLimit.Fallback != "allow" {
		t.Fatalf("RateLimit = %+v", cfg.RateLimit)
	}
	if cfg.Auth.LockoutMaxFailures != 5 || cfg.Auth.LockoutDuration <= 0 {
		t.Fatalf("Auth = %+v", cfg.Auth)
	}
	if cfg.Auth.BcryptCost != bcrypt.DefaultCost {
		t.Fatalf("Auth.BcryptCost = %d", cfg.Auth.BcryptCost)
	}
	if cfg.Auth.RefreshIPAnomalyAction != "audit" {
		t.Fatalf("Auth.RefreshIPAnomalyAction = %q", cfg.Auth.RefreshIPAnomalyAction)
	}
	if !cfg.Errors.IncludeStack {
		t.Fatal("Errors.IncludeStack = false")
	}
	if !cfg.Monitoring.Enabled {
		t.Fatal("Monitoring.Enabled = false")
	}
	if got, want := cfg.Monitoring.AdminRoles, []string{"admin"}; !equalStrings(got, want) {
		t.Fatalf("Monitoring.AdminRoles = %+v, want %+v", got, want)
	}
	if cfg.Monitoring.MetricsCollectInterval != 30*time.Second {
		t.Fatalf("Monitoring.MetricsCollectInterval = %s", cfg.Monitoring.MetricsCollectInterval)
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

func TestValidateRejectsInvalidHTTPTimeouts(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.HTTP.ReadTimeout = 0
	if err := cfg.Validate(); err == nil || err.Error() != "HTTP_READ_TIMEOUT must be positive" {
		t.Fatalf("Validate() error = %v", err)
	}

	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.HTTP.WriteTimeout = 0
	if err := cfg.Validate(); err == nil || err.Error() != "HTTP_WRITE_TIMEOUT must be positive" {
		t.Fatalf("Validate() error = %v", err)
	}

	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.HTTP.IdleTimeout = 0
	if err := cfg.Validate(); err == nil || err.Error() != "HTTP_IDLE_TIMEOUT must be positive" {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestLoadSupportsBcryptCost(t *testing.T) {
	t.Setenv("BCRYPT_COST", "4")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Auth.BcryptCost != bcrypt.MinCost {
		t.Fatalf("Auth.BcryptCost = %d", cfg.Auth.BcryptCost)
	}
}

func TestValidateRejectsInvalidBcryptCost(t *testing.T) {
	t.Setenv("BCRYPT_COST", "3")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil")
	}
	if got, want := err.Error(), "BCRYPT_COST must be between 4 and 31"; got != want {
		t.Fatalf("Load() error = %q, want %q", got, want)
	}
}

func TestLoadRejectsInvalidJWTKeyFormat(t *testing.T) {
	t.Setenv("JWT_ACCESS_CURRENT_KEY", "missing-separator")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil")
	}
	if got, want := err.Error(), "JWT_ACCESS_CURRENT_KEY must use <key-id>/<base64-secret> format"; got != want {
		t.Fatalf("Load() error = %q, want %q", got, want)
	}
}

func TestLoadRejectsPreviousJWTKeyWithoutNotAfter(t *testing.T) {
	t.Setenv("JWT_ACCESS_CURRENT_KEY", keyValue("current", "current-secret-value"))
	t.Setenv("JWT_ACCESS_PREVIOUS_KEY", keyValue("previous", "previous-secret-value"))
	t.Setenv("JWT_ACCESS_PREVIOUS_NOT_AFTER", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil")
	}
	if got, want := err.Error(), "JWT_ACCESS_PREVIOUS_NOT_AFTER must be set when JWT_ACCESS_PREVIOUS_KEY is configured"; got != want {
		t.Fatalf("Load() error = %q, want %q", got, want)
	}
}

func TestLoadRejectsExpiredPreviousJWTKey(t *testing.T) {
	t.Setenv("JWT_ACCESS_CURRENT_KEY", keyValue("current", "current-secret-value"))
	t.Setenv("JWT_ACCESS_PREVIOUS_KEY", keyValue("previous", "previous-secret-value"))
	t.Setenv("JWT_ACCESS_PREVIOUS_NOT_AFTER", time.Now().Add(-time.Minute).Format(time.RFC3339Nano))

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil")
	}
	if got, want := err.Error(), "JWT_ACCESS_PREVIOUS_NOT_AFTER must be in the future when JWT_ACCESS_PREVIOUS_KEY is configured"; got != want {
		t.Fatalf("Load() error = %q, want %q", got, want)
	}
}

func TestLoadSupportsJWTKeyRotationConfig(t *testing.T) {
	t.Setenv("JWT_ACCESS_CURRENT_KEY", keyValue("current", "current-secret-value"))
	t.Setenv("JWT_ACCESS_PREVIOUS_KEY", keyValue("previous", "previous-secret-value"))
	t.Setenv("JWT_ACCESS_PREVIOUS_NOT_AFTER", time.Now().Add(time.Hour).Format(time.RFC3339Nano))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.JWT.AccessCurrentKey == "" || cfg.JWT.AccessPreviousKey == "" || cfg.JWT.PreviousNotAfter.IsZero() {
		t.Fatalf("JWT config = %+v", cfg.JWT)
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

func TestLoadSupportsRateLimitConfig(t *testing.T) {
	t.Setenv("AUTH_RATE_LIMIT_ENABLED", "false")
	t.Setenv("AUTH_RATE_LIMIT_LOGIN_PER_MINUTE", "11")
	t.Setenv("AUTH_RATE_LIMIT_REFRESH_PER_MINUTE", "31")
	t.Setenv("AUTH_RATE_LIMIT_REGISTER_PER_MINUTE", "6")
	t.Setenv("RATE_LIMIT_FALLBACK", "block")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.RateLimit.AuthEnabled {
		t.Fatal("RateLimit.AuthEnabled = true")
	}
	if cfg.RateLimit.LoginPerMinute != 11 || cfg.RateLimit.RefreshPerMinute != 31 || cfg.RateLimit.RegisterPerMinute != 6 {
		t.Fatalf("RateLimit = %+v", cfg.RateLimit)
	}
	if cfg.RateLimit.Fallback != "block" {
		t.Fatalf("RateLimit.Fallback = %q", cfg.RateLimit.Fallback)
	}
}

func TestLoadSupportsRefreshIPAnomalyConfig(t *testing.T) {
	t.Setenv("AUTH_REFRESH_IP_ANOMALY_ACTION", "revoke")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Auth.RefreshIPAnomalyAction != "revoke" {
		t.Fatalf("Auth.RefreshIPAnomalyAction = %q", cfg.Auth.RefreshIPAnomalyAction)
	}
}

func TestValidateRejectsInvalidRefreshIPAnomalyAction(t *testing.T) {
	t.Setenv("AUTH_REFRESH_IP_ANOMALY_ACTION", "invalid")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil")
	}
	if got, want := err.Error(), "AUTH_REFRESH_IP_ANOMALY_ACTION must be audit or revoke"; got != want {
		t.Fatalf("Load() error = %q, want %q", got, want)
	}
}

func TestLoadSupportsRedisTLSConfig(t *testing.T) {
	t.Setenv("REDIS_TLS_ENABLED", "true")
	t.Setenv("REDIS_TLS_CA_CERT", "/tmp/redis-ca.pem")
	t.Setenv("REDIS_TLS_SERVER_NAME", "redis.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Redis.TLSEnabled {
		t.Fatal("Redis.TLSEnabled = false")
	}
	if cfg.Redis.TLSCACert != "/tmp/redis-ca.pem" {
		t.Fatalf("Redis.TLSCACert = %q", cfg.Redis.TLSCACert)
	}
	if cfg.Redis.TLSServerName != "redis.example.com" {
		t.Fatalf("Redis.TLSServerName = %q", cfg.Redis.TLSServerName)
	}
}

func TestValidateRejectsInvalidMongoPoolConfig(t *testing.T) {
	t.Setenv("MONGO_MAX_POOL_SIZE", "0")
	t.Setenv("MONGO_MIN_POOL_SIZE", "1")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil")
	}
	if got, want := err.Error(), "MONGO_MAX_POOL_SIZE must be positive"; got != want {
		t.Fatalf("Load() error = %q, want %q", got, want)
	}
}

func TestValidateRejectsInvalidMongoReadPreference(t *testing.T) {
	t.Setenv("MONGO_READ_PREFERENCE", "random")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil")
	}
	if got, want := err.Error(), "MONGO_READ_PREFERENCE must be one of primary, primaryPreferred, secondary, secondaryPreferred, nearest"; got != want {
		t.Fatalf("Load() error = %q, want %q", got, want)
	}
}

func TestValidateRejectsWildcardCorsInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com,https://*.example.com")

	cfg, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil")
	}
	if got, want := err.Error(), "CORS_ALLOWED_ORIGINS must not contain wildcard in production"; got != want {
		t.Fatalf("Load() error = %q, want %q", got, want)
	}

	if cfg.App.Env != "" {
		t.Fatalf("Config should be zero value on load failure, got %+v", cfg)
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

func TestLoadSupportsETagConfig(t *testing.T) {
	t.Setenv("ETAG_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.ETagEnabled {
		t.Fatal("HTTP.ETagEnabled = true")
	}
}

func TestLoadSupportsCORSConfig(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com")
	t.Setenv("CORS_ALLOWED_METHODS", "GET,POST")
	t.Setenv("CORS_ALLOWED_HEADERS", "Authorization,X-Request-ID")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.HTTP.CORSAllowOrigins, []string{"https://app.example.com"}; !equalStrings(got, want) {
		t.Fatalf("CORSAllowOrigins = %+v, want %+v", got, want)
	}
	if got, want := cfg.HTTP.CORSAllowMethods, []string{"GET", "POST"}; !equalStrings(got, want) {
		t.Fatalf("CORSAllowMethods = %+v, want %+v", got, want)
	}
	if got, want := cfg.HTTP.CORSAllowHeaders, []string{"Authorization", "X-Request-ID"}; !equalStrings(got, want) {
		t.Fatalf("CORSAllowHeaders = %+v, want %+v", got, want)
	}
}

func TestLoadSupportsMonitoringConfig(t *testing.T) {
	t.Setenv("MONITORING_ENABLED", "false")
	t.Setenv("MONITORING_ADMIN_ROLES", "ops,auditor")
	t.Setenv("METRICS_COLLECT_INTERVAL", "45s")
	t.Setenv("PROMETHEUS_ENABLED", "false")
	t.Setenv("PROMETHEUS_PATH", "/prometheus")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Monitoring.Enabled {
		t.Fatal("Monitoring.Enabled = true")
	}
	if got, want := cfg.Monitoring.AdminRoles, []string{"ops", "auditor"}; !equalStrings(got, want) {
		t.Fatalf("Monitoring.AdminRoles = %+v, want %+v", got, want)
	}
	if cfg.Monitoring.MetricsCollectInterval != 45*time.Second {
		t.Fatalf("Monitoring.MetricsCollectInterval = %s", cfg.Monitoring.MetricsCollectInterval)
	}
	if cfg.Metrics.Enabled {
		t.Fatal("Metrics.Enabled = true")
	}
	if cfg.Metrics.Path != "/prometheus" {
		t.Fatalf("Metrics.Path = %q", cfg.Metrics.Path)
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

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func keyValue(id string, secret string) string {
	return id + "/" + base64.RawURLEncoding.EncodeToString([]byte(secret))
}
