package health

import (
	"context"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/monitoring"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type ReadinessConfig struct {
	Timeout                time.Duration
	RequiresRedis          bool
	MongoDegradedThreshold time.Duration
	RedisDegradedThreshold time.Duration
	Now                    func() time.Time
}

type ReadinessChecker struct {
	mongo Pinger
	redis Pinger
	cfg   ReadinessConfig
}

func NewReadinessChecker(mongo Pinger, redis Pinger, cfg ReadinessConfig) *ReadinessChecker {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Second
	}
	if cfg.MongoDegradedThreshold <= 0 {
		cfg.MongoDegradedThreshold = 500 * time.Millisecond
	}
	if cfg.RedisDegradedThreshold <= 0 {
		cfg.RedisDegradedThreshold = 200 * time.Millisecond
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &ReadinessChecker{mongo: mongo, redis: redis, cfg: cfg}
}

func (c *ReadinessChecker) Check(ctx context.Context) (monitoring.DependencyStatus, bool) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	status := monitoring.DependencyStatus{
		MongoDB: c.checkOne(ctx, c.mongo, c.cfg.MongoDegradedThreshold),
		Redis:   c.checkOne(ctx, c.redis, c.cfg.RedisDegradedThreshold),
	}
	ready := status.MongoDB.Status != monitoring.Unhealthy
	if c.cfg.RequiresRedis && status.Redis.Status == monitoring.Unhealthy {
		ready = false
	}
	return status, ready
}

func (c *ReadinessChecker) checkOne(ctx context.Context, pinger Pinger, degradedThreshold time.Duration) monitoring.DependencyCheck {
	checkedAt := c.cfg.Now().UTC()
	if pinger == nil {
		return monitoring.DependencyCheck{
			Status:    monitoring.Unhealthy,
			Error:     "dependency not configured",
			CheckedAt: checkedAt,
		}
	}
	start := c.cfg.Now()
	err := pinger.Ping(ctx)
	latency := c.cfg.Now().Sub(start)
	check := monitoring.DependencyCheck{
		Status:    monitoring.Healthy,
		LatencyMs: latency.Milliseconds(),
		CheckedAt: checkedAt,
	}
	if err != nil {
		check.Status = monitoring.Unhealthy
		check.Error = err.Error()
		return check
	}
	if latency > degradedThreshold {
		check.Status = monitoring.Degraded
	}
	return check
}
