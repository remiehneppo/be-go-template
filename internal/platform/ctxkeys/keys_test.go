package ctxkeys

import (
	"reflect"
	"testing"
)

func TestKeysHaveStableValues(t *testing.T) {
	got := []Key{RequestID, UserID, SessionID, TokenID, Roles, TraceID, SpanID, Logger, RequestStartedAt}
	want := []Key{"request_id", "user_id", "session_id", "token_id", "roles", "trace_id", "span_id", "logger", "request_started_at"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keys = %#v, want %#v", got, want)
	}
	seen := map[Key]struct{}{}
	for _, key := range got {
		if _, ok := seen[key]; ok {
			t.Fatalf("duplicate key %q", key)
		}
		seen[key] = struct{}{}
	}
}
