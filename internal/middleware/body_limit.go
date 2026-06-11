package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
)

func BodyLimit(limitBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limitBytes <= 0 || c.Request.Body == nil {
			c.Next()
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limitBytes)
		if c.Request.ContentLength > limitBytes {
			writeError(c, apperrors.New(apperrors.CodeRequestTooLarge, "Request body too large", http.StatusRequestEntityTooLarge))
			c.Abort()
			return
		}
		c.Next()
	}
}
