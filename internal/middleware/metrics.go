package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/platform/metrics"
)

func Metrics(metricsCollector *metrics.HTTPMetrics, skipPath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if metricsCollector == nil {
			c.Next()
			return
		}
		if skipPath != "" && c.Request.URL.Path == skipPath {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		metricsCollector.Observe(c.Request.Method, route, c.Writer.Status(), time.Since(start))
	}
}
