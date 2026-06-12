package database

import (
	"context"
	"errors"
	"net"

	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
)

func dependencyError(op string, err error) error {
	if err == nil || !isTimeoutError(err) {
		return err
	}
	return apperrors.Dependency(op, err)
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	type timeout interface {
		Timeout() bool
	}
	var timeoutErr timeout
	if errors.As(err, &timeoutErr) && timeoutErr.Timeout() {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}
