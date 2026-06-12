package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

func Logging(log logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		requestLog := logger.FromContext(c.Request.Context())
		if requestLog == nil {
			requestLog = log
		}
		requestLog.Info("http request",
			logger.String("method", c.Request.Method),
			logger.String("path", c.FullPath()),
			logger.String("query", c.Request.URL.RawQuery),
			logger.Int("status", c.Writer.Status()),
			logger.Int("latency_ms", int(time.Since(start).Milliseconds())),
			logger.String("ip", c.ClientIP()),
			logger.String("user_agent", c.Request.UserAgent()),
		)
	}
}
