package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/platform/ctxkeys"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
)

func writeError(c *gin.Context, appErr *apperrors.AppError) {
	status := appErr.HTTPStatus
	if status == 0 {
		status = apperrors.StatusForCode(appErr.Code)
	}
	c.JSON(status, gin.H{
		"success":    false,
		"request_id": requestID(c),
		"error": gin.H{
			"code":    appErr.Code,
			"message": appErr.SafeMessage,
			"details": appErr.Details,
		},
	})
}

func requestID(c *gin.Context) string {
	if value, ok := c.Get(string(ctxkeys.RequestID)); ok {
		if requestID, ok := value.(string); ok {
			return requestID
		}
	}
	if c.Request != nil {
		if requestID, ok := c.Request.Context().Value(ctxkeys.RequestID).(string); ok {
			return requestID
		}
	}
	return ""
}
