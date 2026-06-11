package monitoring

import "time"

type HealthLevel string

const (
	Healthy   HealthLevel = "healthy"
	Degraded  HealthLevel = "degraded"
	Unhealthy HealthLevel = "unhealthy"
)

type SystemStatus struct {
	Status      HealthLevel
	ServiceName string
	Version     string
	CheckedAt   time.Time
}

type RuntimeMetrics struct {
	Goroutines    int
	AllocBytes    uint64
	HeapBytes     uint64
	UptimeSeconds int64
	CollectedAt   time.Time
}

type DependencyStatus struct {
	MongoDB DependencyCheck
	Redis   DependencyCheck
}

type DependencyCheck struct {
	Status    HealthLevel
	LatencyMs int64
	Error     string
	CheckedAt time.Time
}

type AuthStats struct {
	LoginSuccessCount int64
	LoginFailureCount int64
	RefreshCount      int64
	LogoutCount       int64
	From              time.Time
	To                time.Time
}
