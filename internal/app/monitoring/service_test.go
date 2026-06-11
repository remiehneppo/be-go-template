package monitoring

import (
	"context"
	"testing"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/monitoring"
)

func TestServiceRuntimeMetrics(t *testing.T) {
	startedAt := time.Unix(100, 0).UTC()
	now := time.Unix(160, 0).UTC()
	service := NewService(Dependencies{StartedAt: startedAt, Now: func() time.Time { return now }})

	got, err := service.GetRuntimeMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRuntimeMetrics() error = %v", err)
	}
	if got.UptimeSeconds != 60 || got.CollectedAt != now || got.Goroutines <= 0 {
		t.Fatalf("runtime metrics = %+v", got)
	}
}

func TestServiceSystemStatusReflectsDependencies(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	service := NewService(Dependencies{
		ServiceName: "api",
		Version:     "v1",
		Now:         func() time.Time { return now },
		DependencyChecker: &fakeDependencyChecker{
			ready: true,
			status: monitoring.DependencyStatus{
				MongoDB: monitoring.DependencyCheck{Status: monitoring.Healthy},
				Redis:   monitoring.DependencyCheck{Status: monitoring.Unhealthy},
			},
		},
	})

	got, err := service.GetSystemStatus(context.Background())
	if err != nil {
		t.Fatalf("GetSystemStatus() error = %v", err)
	}
	if got.Status != monitoring.Degraded || got.ServiceName != "api" || got.Version != "v1" {
		t.Fatalf("system status = %+v", got)
	}
}

func TestServiceDependencyStatusUsesChecker(t *testing.T) {
	want := monitoring.DependencyStatus{MongoDB: monitoring.DependencyCheck{Status: monitoring.Healthy}}
	service := NewService(Dependencies{DependencyChecker: &fakeDependencyChecker{ready: true, status: want}})

	got, err := service.GetDependencyStatus(context.Background())
	if err != nil {
		t.Fatalf("GetDependencyStatus() error = %v", err)
	}
	if got.MongoDB.Status != monitoring.Healthy {
		t.Fatalf("dependency status = %+v", got)
	}
}

type fakeDependencyChecker struct {
	status monitoring.DependencyStatus
	ready  bool
}

func (c *fakeDependencyChecker) Check(ctx context.Context) (monitoring.DependencyStatus, bool) {
	return c.status, c.ready
}
