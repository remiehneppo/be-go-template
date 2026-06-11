package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestHTTPMetricsObserveRegistersCounters(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewHTTPMetrics(registry, "testapp")
	if err != nil {
		t.Fatalf("NewHTTPMetrics() error = %v", err)
	}

	metrics.Observe("GET", "/healthz", 200, 10*time.Millisecond)

	if err := testutil.GatherAndCompare(registry, strings.NewReader(`
# HELP testapp_http_requests_total Total HTTP requests handled by the API.
# TYPE testapp_http_requests_total counter
testapp_http_requests_total{method="GET",route="/healthz",status="200"} 1
`), "testapp_http_requests_total"); err != nil {
		t.Fatalf("GatherAndCompare() error = %v", err)
	}
}

func TestHTTPMetricsReusesAlreadyRegisteredCollectors(t *testing.T) {
	registry := prometheus.NewRegistry()
	first, err := NewHTTPMetrics(registry, "testapp")
	if err != nil {
		t.Fatalf("NewHTTPMetrics() first error = %v", err)
	}
	second, err := NewHTTPMetrics(registry, "testapp")
	if err != nil {
		t.Fatalf("NewHTTPMetrics() second error = %v", err)
	}

	first.Observe("GET", "/healthz", 200, time.Millisecond)
	second.Observe("GET", "/healthz", 200, time.Millisecond)

	if err := testutil.GatherAndCompare(registry, strings.NewReader(`
# HELP testapp_http_requests_total Total HTTP requests handled by the API.
# TYPE testapp_http_requests_total counter
testapp_http_requests_total{method="GET",route="/healthz",status="200"} 2
`), "testapp_http_requests_total"); err != nil {
		t.Fatalf("GatherAndCompare() error = %v", err)
	}
}
