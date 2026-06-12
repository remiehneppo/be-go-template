package logger

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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

	log.Info("login",
		String("refresh_token", "secret-token"),
		String("password", "password123"),
		String("authorization", "Bearer secret"),
		String("secret_key", "super-secret"),
		String("email", "user@example.com"),
	)

	output := buf.String()
	for _, leaked := range []string{"secret-token", "password123", "Bearer secret", "super-secret"} {
		if strings.Contains(output, leaked) {
			t.Fatalf("log output leaked secret %q: %s", leaked, output)
		}
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

func TestLoggerWithCarriesFields(t *testing.T) {
	buf := bytes.Buffer{}
	base := &structuredLogger{
		level:  parseLevel("debug"),
		format: "json",
		writer: &buf,
		mu:     &sync.Mutex{},
	}

	base.With(String("request_id", "req-1")).Info("hello", String("user_id", "u1"))

	output := buf.String()
	if !strings.Contains(output, "req-1") || !strings.Contains(output, "u1") {
		t.Fatalf("log output missing carried fields: %s", output)
	}
}

func TestLoggerWritesRotatingFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "app.log")

	log, closeFn, err := New(Config{
		Level:      "info",
		Format:     "json",
		FilePath:   filePath,
		ToFile:     true,
		MaxSizeMB:  1,
		MaxBackups: 1,
		MaxAgeDays: 1,
		Compress:   false,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	payload := strings.Repeat("x", 4*1024)
	for i := 0; i < 300; i++ {
		log.Info("rotate", String("payload", payload))
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close logger: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "app*"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(files) < 2 {
		t.Fatalf("expected rotated files, got %v", files)
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("primary log file missing: %v", err)
	}
}

func TestLoggerTextFormat(t *testing.T) {
	buf := bytes.Buffer{}
	log := &structuredLogger{
		level:  parseLevel("debug"),
		format: "text",
		writer: &buf,
		mu:     &sync.Mutex{},
	}

	log.Info("hello", String("user_id", "u1"))

	output := buf.String()
	if !strings.Contains(output, `level="info"`) || !strings.Contains(output, `msg="hello"`) || !strings.Contains(output, `user_id="u1"`) {
		t.Fatalf("unexpected text output: %s", output)
	}
}
