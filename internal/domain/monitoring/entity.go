// Package monitoring defines the entity types for the admin monitoring service.
package monitoring

import "time"

// HealthLevel represents the health status of a dependency.
type HealthLevel string

const (
	// Healthy indicates the dependency is operating normally.
	Healthy HealthLevel = "healthy"
	// Degraded indicates the dependency is responding slowly.
	Degraded HealthLevel = "degraded"
	// Unhealthy indicates the dependency is unavailable.
	Unhealthy HealthLevel = "unhealthy"
)

// SystemStatus represents the application health snapshot returned by the
// /v1/admin/monitoring/status endpoint.
type SystemStatus struct {
	Status        HealthLevel `json:"status"`
	ServiceName   string      `json:"service_name"`
	Version       string      `json:"version"`
	Env           string      `json:"env"`
	StartedAt     time.Time   `json:"started_at"`
	UptimeSeconds int64       `json:"uptime_seconds"`
	CheckedAt     time.Time   `json:"checked_at"`
}

// RuntimeMetrics represents Go process metrics returned by the
// /v1/admin/monitoring/runtime endpoint.
type RuntimeMetrics struct {
	Goroutines    int       `json:"goroutines"`
	AllocBytes    uint64    `json:"alloc_bytes"`
	HeapBytes     uint64    `json:"heap_bytes"`
	NumGC         uint32    `json:"num_gc"`
	UptimeSeconds int64     `json:"uptime_seconds"`
	CollectedAt   time.Time `json:"collected_at"`
}

// DependencyStatus represents the health of downstream dependencies (MongoDB, Redis)
// returned by the /v1/admin/monitoring/dependencies endpoint.
type DependencyStatus struct {
	MongoDB DependencyCheck `json:"mongodb"`
	Redis   DependencyCheck `json:"redis"`
}

// DependencyCheck holds latency and health for a single dependency.
type DependencyCheck struct {
	Status    HealthLevel `json:"status"`
	LatencyMs int64       `json:"latency_ms"`
	Error     string      `json:"error,omitempty"`
	CheckedAt time.Time   `json:"checked_at"`
}

// AuthStats represents auth-specific counters returned by the
// /v1/admin/monitoring/auth-stats endpoint.
type AuthStats struct {
	LoginSuccessCount   int64     `json:"login_success_count"`
	LoginFailureCount   int64     `json:"login_failure_count"`
	ActiveSessionCount  int64     `json:"active_session_count"`
	RevokedSessionCount int64     `json:"revoked_session_count"`
	RefreshCount        int64     `json:"refresh_count"`
	LogoutCount         int64     `json:"logout_count"`
	From                time.Time `json:"from"`
	To                  time.Time `json:"to"`
}
