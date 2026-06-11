package logger

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
)

func TestLoggerRedactsSensitiveFields(t *testing.T) {
	buf := bytes.Buffer{}
	log := &structuredLogger{
		level:  parseLevel("debug"),
		writer: &buf,
		mu:     &sync.Mutex{},
	}

	log.Info("login", String("refresh_token", "secret-token"), String("email", "user@example.com"))

	output := buf.String()
	if strings.Contains(output, "secret-token") {
		t.Fatalf("log output leaked token: %s", output)
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Fatalf("log output did not redact token: %s", output)
	}
}

func TestContextLoggerRoundTrip(t *testing.T) {
	log := noopLogger{}
	ctx := WithContext(context.Background(), log)

	got := FromContext(ctx)
	if got == nil {
		t.Fatal("FromContext() = nil")
	}
}
