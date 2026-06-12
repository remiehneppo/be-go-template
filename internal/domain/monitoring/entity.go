package monitoring

import "time"

type HealthLevel string

const (
	Healthy   HealthLevel = "healthy"
	Degraded  HealthLevel = "degraded"
	Unhealthy HealthLevel = "unhealthy"
)

type SystemStatus struct {
	Status        HealthLevel `json:"status"`
	ServiceName   string      `json:"service_name"`
	Version       string      `json:"version"`
	Env           string      `json:"env"`
	StartedAt     time.Time   `json:"started_at"`
	UptimeSeconds int64       `json:"uptime_seconds"`
	CheckedAt     time.Time   `json:"checked_at"`
}

type RuntimeMetrics struct {
	Goroutines    int       `json:"goroutines"`
	AllocBytes    uint64    `json:"alloc_bytes"`
	HeapBytes     uint64    `json:"heap_bytes"`
	NumGC         uint32    `json:"num_gc"`
	UptimeSeconds int64     `json:"uptime_seconds"`
	CollectedAt   time.Time `json:"collected_at"`
}

type DependencyStatus struct {
	MongoDB DependencyCheck `json:"mongodb"`
	Redis   DependencyCheck `json:"redis"`
}

type DependencyCheck struct {
	Status    HealthLevel `json:"status"`
	LatencyMs int64       `json:"latency_ms"`
	Error     string      `json:"error,omitempty"`
	CheckedAt time.Time   `json:"checked_at"`
}

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
