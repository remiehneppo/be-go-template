package monitoring

import (
	"context"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
)

type Service interface {
	GetSystemStatus(ctx context.Context) (*SystemStatus, error)
	GetRuntimeMetrics(ctx context.Context) (*RuntimeMetrics, error)
	GetDependencyStatus(ctx context.Context) (*DependencyStatus, error)
	GetAuthStats(ctx context.Context, from time.Time, to time.Time) (*AuthStats, error)
	GetRecentErrors(ctx context.Context, pagination common.Pagination) ([]auth.ErrorEvent, error)
	GetRecentAuditLogs(ctx context.Context, pagination common.Pagination) ([]auth.AuditLog, error)
}
