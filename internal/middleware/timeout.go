package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
)

func Timeout(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if timeout <= 0 {
			c.Next()
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
		if ctx.Err() == context.DeadlineExceeded && !c.Writer.Written() {
			writeError(c, apperrors.New(apperrors.CodeTimeout, "Request timed out", http.StatusGatewayTimeout))
			c.Abort()
		}
	}
}
