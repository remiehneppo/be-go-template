package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestDatabaseMetricsRecordEvents(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewDatabaseMetrics(registry, "testapp")
	if err != nil {
		t.Fatalf("NewDatabaseMetrics() error = %v", err)
	}

	metrics.RecordCacheEvent("read", "hit")
	metrics.RecordCacheEvent("read", "miss")
	metrics.RecordDependencyError("CachedDatabase.FindOne")
	metrics.ObserveCacheLock("read", 250*time.Millisecond)

	if err := testutil.GatherAndCompare(registry, strings.NewReader(`
# HELP testapp_database_cache_events_total Total database cache events grouped by operation and result.
# TYPE testapp_database_cache_events_total counter
testapp_database_cache_events_total{operation="read",result="hit"} 1
testapp_database_cache_events_total{operation="read",result="miss"} 1
# HELP testapp_database_dependency_errors_total Total dependency errors observed in the database abstraction.
# TYPE testapp_database_dependency_errors_total counter
testapp_database_dependency_errors_total{operation="CachedDatabase.FindOne"} 1
`), "testapp_database_cache_events_total", "testapp_database_dependency_errors_total"); err != nil {
		t.Fatalf("GatherAndCompare() error = %v", err)
	}
}

func TestDatabaseMetricsReusesAlreadyRegisteredCollectors(t *testing.T) {
	registry := prometheus.NewRegistry()
	first, err := NewDatabaseMetrics(registry, "testapp")
	if err != nil {
		t.Fatalf("NewDatabaseMetrics() first error = %v", err)
	}
	second, err := NewDatabaseMetrics(registry, "testapp")
	if err != nil {
		t.Fatalf("NewDatabaseMetrics() second error = %v", err)
	}

	first.RecordCacheEvent("read", "hit")
	second.RecordCacheEvent("read", "hit")
	if err := testutil.GatherAndCompare(registry, strings.NewReader(`
# HELP testapp_database_cache_events_total Total database cache events grouped by operation and result.
# TYPE testapp_database_cache_events_total counter
testapp_database_cache_events_total{operation="read",result="hit"} 2
`), "testapp_database_cache_events_total"); err != nil {
		t.Fatalf("GatherAndCompare() error = %v", err)
	}
}
