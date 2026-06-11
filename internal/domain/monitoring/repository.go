package monitoring

import (
	"context"

	"github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
)

type ErrorEventRepository interface {
	Append(ctx context.Context, event auth.ErrorEvent) error
	List(ctx context.Context, pagination common.Pagination) ([]auth.ErrorEvent, error)
}
