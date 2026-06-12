package monitoring

import (
	"context"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
)

type ErrorEventRepository interface {
	Append(ctx context.Context, event auth.ErrorEvent) error
	List(ctx context.Context, filter auth.ErrorEventFilter, pagination common.Pagination) ([]auth.ErrorEvent, error)
}

type AuthStatsRepository interface {
	GetAuthStats(ctx context.Context, from time.Time, to time.Time) (*AuthStats, error)
}
