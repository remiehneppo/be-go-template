package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type DatabaseMetrics struct {
	cacheEvents       *prometheus.CounterVec
	cacheLockDuration *prometheus.HistogramVec
	dependencyErrors  *prometheus.CounterVec
}

func NewDatabaseMetrics(registerer prometheus.Registerer, namespace string) (*DatabaseMetrics, error) {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	if namespace == "" {
		namespace = "be_go_template"
	}
	m := &DatabaseMetrics{
		cacheEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "database_cache_events_total",
			Help:      "Total database cache events grouped by operation and result.",
		}, []string{"operation", "result"}),
		cacheLockDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "database_cache_lock_seconds",
			Help:      "Duration of cache-protected database operations.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"operation"}),
		dependencyErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "database_dependency_errors_total",
			Help:      "Total dependency errors observed in the database abstraction.",
		}, []string{"operation"}),
	}
	var err error
	if m.cacheEvents, err = registerCounterVec(registerer, m.cacheEvents); err != nil {
		return nil, err
	}
	if m.cacheLockDuration, err = registerHistogramVec(registerer, m.cacheLockDuration); err != nil {
		return nil, err
	}
	if m.dependencyErrors, err = registerCounterVec(registerer, m.dependencyErrors); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *DatabaseMetrics) RecordCacheEvent(operation, result string) {
	if m == nil {
		return
	}
	if operation == "" {
		operation = "unknown"
	}
	if result == "" {
		result = "unknown"
	}
	m.cacheEvents.WithLabelValues(operation, result).Inc()
}

func (m *DatabaseMetrics) ObserveCacheLock(operation string, duration time.Duration) {
	if m == nil {
		return
	}
	if operation == "" {
		operation = "unknown"
	}
	m.cacheLockDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

func (m *DatabaseMetrics) RecordDependencyError(operation string) {
	if m == nil {
		return
	}
	if operation == "" {
		operation = "unknown"
	}
	m.dependencyErrors.WithLabelValues(operation).Inc()
}
