package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/platform/ctxkeys"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

func Recovery(log logger.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		appErr := apperrors.New(apperrors.CodeInternal, "Internal server error", http.StatusInternalServerError)
		appErr.Stack = debug.Stack()
		requestLog := logger.FromContext(c.Request.Context())
		requestLog.Error("panic recovered",
			logger.Any("panic", recovered),
			logger.String("request_id", requestID(c)),
			logger.String("user_id", contextString(c, ctxkeys.UserID)),
			logger.String("session_id", contextString(c, ctxkeys.SessionID)),
			logger.String("method", c.Request.Method),
			logger.String("path", c.Request.URL.Path),
			logger.Int("status", appErr.HTTPStatus),
			logger.String("error_code", string(appErr.Code)),
			logger.String("stack", string(appErr.Stack)),
		)
		writeError(c, appErr)
		c.Abort()
	})
}
