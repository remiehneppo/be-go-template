package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	platformmetrics "github.com/remihneppo/be-go-template/internal/platform/metrics"
)

func TestMetricsMiddlewareRecordsRouteAndStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := prometheus.NewRegistry()
	metricsCollector, err := platformmetrics.NewHTTPMetrics(registry, "testapp")
	if err != nil {
		t.Fatalf("NewHTTPMetrics() error = %v", err)
	}

	router := gin.New()
	router.Use(Metrics(metricsCollector, "/metrics"))
	router.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if err := testutil.GatherAndCompare(registry, strings.NewReader(`
# HELP testapp_http_requests_total Total HTTP requests handled by the API.
# TYPE testapp_http_requests_total counter
testapp_http_requests_total{method="GET",route="/healthz",status="204"} 1
`), "testapp_http_requests_total"); err != nil {
		t.Fatalf("GatherAndCompare() error = %v", err)
	}
}
