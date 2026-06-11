package middleware

import (
	"github.com/gin-gonic/gin"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

func ErrorHandler(log logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}
		err := c.Errors.Last().Err
		logger.FromContext(c.Request.Context()).Error("request failed", logger.Any("error", err))
		writeError(c, apperrors.FromError(err))
	}
}
