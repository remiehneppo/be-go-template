package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestAuthMetricsRecordCountersAndGauge(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewAuthMetrics(registry, "testapp")
	if err != nil {
		t.Fatalf("NewAuthMetrics() error = %v", err)
	}

	metrics.RecordLogin(true)
	metrics.RecordLogin(false)
	metrics.RecordRefresh(true)
	metrics.RecordRefreshReuseSuspected()
	metrics.RecordLogout()
	metrics.RecordSessionCreated()
	metrics.RecordSessionRevoked(1)

	if err := testutil.GatherAndCompare(registry, strings.NewReader(`
# HELP testapp_auth_login_total Total auth login attempts grouped by outcome.
# TYPE testapp_auth_login_total counter
testapp_auth_login_total{result="failure"} 1
testapp_auth_login_total{result="success"} 1
# HELP testapp_auth_refresh_total Total auth refresh attempts grouped by outcome.
# TYPE testapp_auth_refresh_total counter
testapp_auth_refresh_total{result="reuse_suspected"} 1
testapp_auth_refresh_total{result="success"} 1
# HELP testapp_auth_logout_total Total auth logout operations handled by the API.
# TYPE testapp_auth_logout_total counter
testapp_auth_logout_total{kind="logout"} 1
# HELP testapp_auth_session_events_total Total auth session create and revoke events.
# TYPE testapp_auth_session_events_total counter
testapp_auth_session_events_total{kind="created"} 1
testapp_auth_session_events_total{kind="revoked"} 1
# HELP testapp_auth_active_sessions Current active auth sessions tracked by the API.
# TYPE testapp_auth_active_sessions gauge
testapp_auth_active_sessions 0
`), "testapp_auth_login_total", "testapp_auth_refresh_total", "testapp_auth_logout_total", "testapp_auth_session_events_total", "testapp_auth_active_sessions"); err != nil {
		t.Fatalf("GatherAndCompare() error = %v", err)
	}
}

func TestAuthMetricsReusesAlreadyRegisteredCollectors(t *testing.T) {
	registry := prometheus.NewRegistry()
	first, err := NewAuthMetrics(registry, "testapp")
	if err != nil {
		t.Fatalf("NewAuthMetrics() first error = %v", err)
	}
	second, err := NewAuthMetrics(registry, "testapp")
	if err != nil {
		t.Fatalf("NewAuthMetrics() second error = %v", err)
	}

	first.RecordLogin(true)
	second.RecordLogin(true)
	if err := testutil.GatherAndCompare(registry, strings.NewReader(`
# HELP testapp_auth_login_total Total auth login attempts grouped by outcome.
# TYPE testapp_auth_login_total counter
testapp_auth_login_total{result="success"} 2
`), "testapp_auth_login_total"); err != nil {
		t.Fatalf("GatherAndCompare() error = %v", err)
	}
}
