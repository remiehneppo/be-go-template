package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type AuthMetrics struct {
	loginEvents         *prometheus.CounterVec
	refreshEvents       *prometheus.CounterVec
	logoutEvents        *prometheus.CounterVec
	sessionEvents       *prometheus.CounterVec
	activeSessionsGauge prometheus.Gauge
}

func NewAuthMetrics(registerer prometheus.Registerer, namespace string) (*AuthMetrics, error) {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	if namespace == "" {
		namespace = "be_go_template"
	}
	m := &AuthMetrics{
		loginEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "auth_login_total",
			Help:      "Total auth login attempts grouped by outcome.",
		}, []string{"result"}),
		refreshEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "auth_refresh_total",
			Help:      "Total auth refresh attempts grouped by outcome.",
		}, []string{"result"}),
		logoutEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "auth_logout_total",
			Help:      "Total auth logout operations handled by the API.",
		}, []string{"kind"}),
		sessionEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "auth_session_events_total",
			Help:      "Total auth session create and revoke events.",
		}, []string{"kind"}),
		activeSessionsGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "auth_active_sessions",
			Help:      "Current active auth sessions tracked by the API.",
		}),
	}
	var err error
	if m.loginEvents, err = registerCounterVec(registerer, m.loginEvents); err != nil {
		return nil, err
	}
	if m.refreshEvents, err = registerCounterVec(registerer, m.refreshEvents); err != nil {
		return nil, err
	}
	if m.logoutEvents, err = registerCounterVec(registerer, m.logoutEvents); err != nil {
		return nil, err
	}
	if m.sessionEvents, err = registerCounterVec(registerer, m.sessionEvents); err != nil {
		return nil, err
	}
	if err := registerer.Register(m.activeSessionsGauge); err != nil {
		if alreadyRegistered, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := alreadyRegistered.ExistingCollector.(prometheus.Gauge); ok {
				m.activeSessionsGauge = existing
				return m, nil
			}
		}
		return nil, err
	}
	return m, nil
}

func (m *AuthMetrics) RecordLogin(success bool) {
	if m == nil {
		return
	}
	m.loginEvents.WithLabelValues(boolOutcome(success)).Inc()
}

func (m *AuthMetrics) RecordRefresh(success bool) {
	if m == nil {
		return
	}
	m.refreshEvents.WithLabelValues(boolOutcome(success)).Inc()
}

func (m *AuthMetrics) RecordLogout() {
	if m == nil {
		return
	}
	m.logoutEvents.WithLabelValues("logout").Inc()
}

func (m *AuthMetrics) RecordSessionCreated() {
	if m == nil {
		return
	}
	m.sessionEvents.WithLabelValues("created").Inc()
	m.activeSessionsGauge.Inc()
}

func (m *AuthMetrics) RecordSessionRevoked(count int64) {
	if m == nil || count <= 0 {
		return
	}
	m.sessionEvents.WithLabelValues("revoked").Add(float64(count))
	m.activeSessionsGauge.Sub(float64(count))
}

func (m *AuthMetrics) RecordRefreshReuseSuspected() {
	if m == nil {
		return
	}
	m.refreshEvents.WithLabelValues("reuse_suspected").Inc()
}

func boolOutcome(success bool) string {
	if success {
		return "success"
	}
	return "failure"
}
