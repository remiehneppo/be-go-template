package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/platform/ctxkeys"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

type ErrorEventReporter interface {
	Append(ctx context.Context, event auth.ErrorEvent) error
}

func ErrorHandler(log logger.Logger, reporters ...ErrorEventReporter) gin.HandlerFunc {
	var reporter ErrorEventReporter
	if len(reporters) > 0 {
		reporter = reporters[0]
	}
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}
		err := c.Errors.Last().Err
		appErr := apperrors.FromError(err)
		logger.FromContext(c.Request.Context()).Error("request failed", logger.Any("error", err))
		reportErrorEvent(c, reporter, appErr)
		writeError(c, appErr)
	}
}

func reportErrorEvent(c *gin.Context, reporter ErrorEventReporter, appErr *apperrors.AppError) {
	if reporter == nil || appErr == nil || c.Request == nil {
		return
	}
	status := appErr.HTTPStatus
	if status == 0 {
		status = apperrors.StatusForCode(appErr.Code)
	}
	event := auth.ErrorEvent{
		RequestID: requestID(c),
		ErrorCode: string(appErr.Code),
		Message:   appErr.SafeMessage,
		Cause:     appErr.Message,
		Stack:     string(appErr.Stack),
		Path:      c.Request.URL.Path,
		Method:    c.Request.Method,
		Status:    status,
		UserID:    contextString(c, ctxkeys.UserID),
		CreatedAt: time.Now().UTC(),
	}
	if err := reporter.Append(c.Request.Context(), event); err != nil {
		logger.FromContext(c.Request.Context()).Warn("append error event failed", logger.Any("error", err))
	}
}

func contextString(c *gin.Context, key ctxkeys.Key) string {
	if value, ok := c.Get(string(key)); ok {
		if text, ok := value.(string); ok {
			return text
		}
	}
	if c.Request != nil {
		if text, ok := c.Request.Context().Value(key).(string); ok {
			return text
		}
	}
	return ""
}
