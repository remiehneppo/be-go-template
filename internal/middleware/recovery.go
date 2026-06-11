package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

func Recovery(log logger.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.FromContext(c.Request.Context()).Error("panic recovered", logger.Any("panic", recovered))
		writeError(c, apperrors.New(apperrors.CodeInternal, "Internal server error", http.StatusInternalServerError))
		c.Abort()
	})
}
