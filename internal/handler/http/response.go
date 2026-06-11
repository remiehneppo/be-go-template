package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/platform/ctxkeys"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
)

type Response struct {
	Success   bool   `json:"success"`
	RequestID string `json:"request_id,omitempty"`
	Data      any    `json:"data,omitempty"`
	Error     any    `json:"error,omitempty"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Success:   true,
		RequestID: RequestID(c),
		Data:      data,
	})
}

func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Response{
		Success:   true,
		RequestID: RequestID(c),
		Data:      data,
	})
}

func Error(c *gin.Context, err error) {
	appErr := apperrors.FromError(err)
	status := appErr.HTTPStatus
	if status == 0 {
		status = apperrors.StatusForCode(appErr.Code)
	}
	c.JSON(status, Response{
		Success:   false,
		RequestID: RequestID(c),
		Error: gin.H{
			"code":    appErr.Code,
			"message": appErr.SafeMessage,
			"details": appErr.Details,
		},
	})
}

func RequestID(c *gin.Context) string {
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
