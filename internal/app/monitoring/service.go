package monitoring

import (
	"context"
	"runtime"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	domainmonitoring "github.com/remihneppo/be-go-template/internal/domain/monitoring"
	"github.com/remihneppo/be-go-template/internal/platform/cache"
)

type DependencyChecker interface {
	Check(ctx context.Context) (domainmonitoring.DependencyStatus, bool)
}

type Dependencies struct {
	ServiceName       string
	Version           string
	StartedAt         time.Time
	DependencyChecker DependencyChecker
	AuthStats         domainmonitoring.AuthStatsRepository
	AuditLogs         auth.AuditLogRepository
	ErrorEvents       domainmonitoring.ErrorEventRepository
	Cache             cache.Cache
	AuthStatsTTL      time.Duration
	Now               func() time.Time
}

type Service struct {
	serviceName       string
	version           string
	startedAt         time.Time
	dependencyChecker DependencyChecker
	authStats         domainmonitoring.AuthStatsRepository
	auditLogs         auth.AuditLogRepository
	errorEvents       domainmonitoring.ErrorEventRepository
	cache             cache.Cache
	authStatsTTL      time.Duration
	now               func() time.Time
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	startedAt := deps.StartedAt
	if startedAt.IsZero() {
		startedAt = now()
	}
	serviceName := deps.ServiceName
	if serviceName == "" {
		serviceName = "be-go-template"
	}
	version := deps.Version
	if version == "" {
		version = "dev"
	}
	statsTTL := deps.AuthStatsTTL
	if statsTTL <= 0 {
		statsTTL = 30 * time.Second
	}
	return &Service{
		serviceName:       serviceName,
		version:           version,
		startedAt:         startedAt,
		dependencyChecker: deps.DependencyChecker,
		authStats:         deps.AuthStats,
		auditLogs:         deps.AuditLogs,
		errorEvents:       deps.ErrorEvents,
		cache:             deps.Cache,
		authStatsTTL:      statsTTL,
		now:               now,
	}
}

func (s *Service) GetSystemStatus(ctx context.Context) (*domainmonitoring.SystemStatus, error) {
	status := domainmonitoring.Healthy
	if s.dependencyChecker != nil {
		dependencies, ready := s.dependencyChecker.Check(ctx)
		if !ready {
			status = domainmonitoring.Unhealthy
		} else if dependencies.MongoDB.Status == domainmonitoring.Degraded ||
			dependencies.Redis.Status == domainmonitoring.Degraded ||
			dependencies.Redis.Status == domainmonitoring.Unhealthy {
			status = domainmonitoring.Degraded
		}
	}
	return &domainmonitoring.SystemStatus{
		Status:      status,
		ServiceName: s.serviceName,
		Version:     s.version,
		CheckedAt:   s.now(),
	}, nil
}

func (s *Service) GetRuntimeMetrics(ctx context.Context) (*domainmonitoring.RuntimeMetrics, error) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	now := s.now()
	return &domainmonitoring.RuntimeMetrics{
		Goroutines:    runtime.NumGoroutine(),
		AllocBytes:    mem.Alloc,
		HeapBytes:     mem.HeapAlloc,
		NumGC:         mem.NumGC,
		UptimeSeconds: int64(now.Sub(s.startedAt).Seconds()),
		CollectedAt:   now,
	}, nil
}

func (s *Service) GetDependencyStatus(ctx context.Context) (*domainmonitoring.DependencyStatus, error) {
	if s.dependencyChecker == nil {
		now := s.now()
		return &domainmonitoring.DependencyStatus{
			MongoDB: domainmonitoring.DependencyCheck{Status: domainmonitoring.Unhealthy, Error: "dependency checker not configured", CheckedAt: now},
			Redis:   domainmonitoring.DependencyCheck{Status: domainmonitoring.Unhealthy, Error: "dependency checker not configured", CheckedAt: now},
		}, nil
	}
	status, _ := s.dependencyChecker.Check(ctx)
	return &status, nil
}

func (s *Service) GetAuthStats(ctx context.Context, from time.Time, to time.Time) (*domainmonitoring.AuthStats, error) {
	if s.cache != nil {
		key := authStatsCacheKey(from, to)
		var cached domainmonitoring.AuthStats
		if err := s.cache.Get(ctx, key, &cached); err == nil {
			return &cached, nil
		}
	}
	if s.authStats != nil {
		stats, err := s.authStats.GetAuthStats(ctx, from, to)
		if err != nil {
			return nil, err
		}
		if s.cache != nil && stats != nil {
			ignoreError(s.cache.Set(ctx, authStatsCacheKey(from, to), stats, s.authStatsTTL))
		}
		return stats, nil
	}
	return &domainmonitoring.AuthStats{From: from, To: to}, nil
}

func (s *Service) GetRecentErrors(ctx context.Context, filter auth.ErrorEventFilter, pagination common.Pagination) ([]auth.ErrorEvent, error) {
	if s.errorEvents == nil {
		return []auth.ErrorEvent{}, nil
	}
	return s.errorEvents.List(ctx, filter, pagination.Normalized(20, 100))
}

func (s *Service) GetRecentAuditLogs(ctx context.Context, filter auth.AuditLogFilter, pagination common.Pagination) ([]auth.AuditLog, error) {
	if s.auditLogs == nil {
		return []auth.AuditLog{}, nil
	}
	return s.auditLogs.List(ctx, filter, pagination.Normalized(20, 100))
}

func authStatsCacheKey(from time.Time, to time.Time) string {
	return "monitoring:auth_stats:" + strconvFormatTime(from) + ":" + strconvFormatTime(to)
}

func strconvFormatTime(value time.Time) string {
	if value.IsZero() {
		return "0"
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func ignoreError(err error) {
}

var _ domainmonitoring.Service = (*Service)(nil)
