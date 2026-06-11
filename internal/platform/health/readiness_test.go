package health

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/monitoring"
)

func TestReadinessCheckerRequiresMongo(t *testing.T) {
	checker := NewReadinessChecker(&fakePinger{}, &fakePinger{}, ReadinessConfig{})

	status, ready := checker.Check(context.Background())

	if !ready {
		t.Fatalf("ready = false, status = %+v", status)
	}
	if status.MongoDB.Status != monitoring.Healthy || status.Redis.Status != monitoring.Healthy {
		t.Fatalf("status = %+v", status)
	}
}

func TestReadinessCheckerFailsWhenMongoUnhealthy(t *testing.T) {
	checker := NewReadinessChecker(&fakePinger{err: errors.New("mongo down")}, &fakePinger{}, ReadinessConfig{})

	status, ready := checker.Check(context.Background())

	if ready {
		t.Fatalf("ready = true, status = %+v", status)
	}
	if status.MongoDB.Status != monitoring.Unhealthy {
		t.Fatalf("mongo status = %+v", status.MongoDB)
	}
}

func TestReadinessCheckerRedisCanBeDegradedWhenNotRequired(t *testing.T) {
	checker := NewReadinessChecker(&fakePinger{}, &fakePinger{err: errors.New("redis down")}, ReadinessConfig{})

	status, ready := checker.Check(context.Background())

	if !ready {
		t.Fatalf("ready = false, status = %+v", status)
	}
	if status.Redis.Status != monitoring.Unhealthy {
		t.Fatalf("redis status = %+v", status.Redis)
	}
}

func TestReadinessCheckerRedisCanBeRequired(t *testing.T) {
	checker := NewReadinessChecker(&fakePinger{}, &fakePinger{err: errors.New("redis down")}, ReadinessConfig{RequiresRedis: true})

	status, ready := checker.Check(context.Background())

	if ready {
		t.Fatalf("ready = true, status = %+v", status)
	}
}

func TestReadinessCheckerMarksSlowDependencyAsDegraded(t *testing.T) {
	now := time.Unix(100, 0)
	checker := NewReadinessChecker(&fakePinger{}, &fakePinger{}, ReadinessConfig{
		MongoDegradedThreshold: time.Millisecond,
		RedisDegradedThreshold: time.Second,
		Now: func() time.Time {
			current := now
			now = now.Add(2 * time.Millisecond)
			return current
		},
	})

	status, ready := checker.Check(context.Background())

	if !ready {
		t.Fatalf("ready = false, status = %+v", status)
	}
	if status.MongoDB.Status != monitoring.Degraded {
		t.Fatalf("mongo status = %+v", status.MongoDB)
	}
}

type fakePinger struct {
	err error
}

func (p *fakePinger) Ping(ctx context.Context) error {
	if p.err != nil {
		return p.err
	}
	return nil
}
