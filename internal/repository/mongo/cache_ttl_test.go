package mongo

import (
	"testing"
	"time"
)

func TestCacheTTLsStayWithinPolicy(t *testing.T) {
	cases := []struct {
		name string
		ttl  time.Duration
		min  time.Duration
		max  time.Duration
	}{
		{name: "user profile", ttl: userProfileCacheTTL, min: 5 * time.Minute, max: 15 * time.Minute},
		{name: "session active", ttl: sessionActiveCacheTTL, min: 1 * time.Minute, max: 5 * time.Minute},
		{name: "session refresh", ttl: sessionRefreshCacheTTL, min: 30 * time.Second, max: 120 * time.Second},
		{name: "active sessions", ttl: activeSessionsCacheTTL, min: 30 * time.Second, max: 120 * time.Second},
	}

	for _, tc := range cases {
		if tc.ttl < tc.min || tc.ttl > tc.max {
			t.Fatalf("%s ttl = %s want between %s and %s", tc.name, tc.ttl, tc.min, tc.max)
		}
	}
}
