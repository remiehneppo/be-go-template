package middleware

import (
	"context"
	"net/http"
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
		log := logger.FromContext(c.Request.Context())
		fields := errorLogFields(c, appErr, err)
		if isClientError(appErr.HTTPStatus) {
			log.Warn("request failed", fields...)
		} else {
			log.Error("request failed", fields...)
		}
		reportErrorEvent(c, reporter, appErr)
		writeError(c, appErr)
	}
}

func errorLogFields(c *gin.Context, appErr *apperrors.AppError, err error) []logger.Field {
	status := appErr.HTTPStatus
	if status == 0 {
		status = apperrors.StatusForCode(appErr.Code)
	}
	fields := []logger.Field{
		logger.String("request_id", requestID(c)),
		logger.String("user_id", contextString(c, ctxkeys.UserID)),
		logger.String("session_id", contextString(c, ctxkeys.SessionID)),
		logger.String("method", c.Request.Method),
		logger.String("path", c.Request.URL.Path),
		logger.Int("status", status),
		logger.String("error_code", string(appErr.Code)),
		logger.Any("error", err),
		logger.Any("cause", appErr.Cause),
		logger.Any("retryable", appErr.Retryable),
	}
	if latency := requestLatency(c); latency >= 0 {
		fields = append(fields, logger.Int("latency_ms", latency))
	}
	if appErr.Stack != nil && len(appErr.Stack) > 0 {
		fields = append(fields, logger.String("stack", string(appErr.Stack)))
	}
	return fields
}

func isClientError(status int) bool {
	return status > 0 && status < http.StatusInternalServerError
}

func requestLatency(c *gin.Context) int {
	startedAt := contextTime(c, ctxkeys.RequestStartedAt)
	if startedAt.IsZero() {
		return -1
	}
	return int(time.Since(startedAt).Milliseconds())
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

func contextTime(c *gin.Context, key ctxkeys.Key) time.Time {
	if value, ok := c.Get(string(key)); ok {
		if ts, ok := value.(time.Time); ok {
			return ts
		}
	}
	if c.Request != nil {
		if ts, ok := c.Request.Context().Value(key).(time.Time); ok {
			return ts
		}
	}
	return time.Time{}
}
