// Package logger provides structured logging with JSON or text format,
// optional file output with rotation, and automatic redaction of sensitive
// fields such as passwords, tokens, and secrets.
//
// Use New() to create a logger configured through environment variables.
// The returned CloseFunc should be called during shutdown to flush file logs.
package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/remihneppo/be-go-template/internal/platform/ctxkeys"
)

// Field represents a key-value pair attached to a log entry.
type Field struct {
	Key   string
	Value any
}

// Logger writes structured log entries at debug, info, warn, and error level.
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	With(fields ...Field) Logger
}

// Config controls logger behaviour: level, format, file path, rotation knobs,
// and output toggles for terminal and file.
type Config struct {
	Level      string
	Format     string
	FilePath   string
	ToTerminal bool
	ToFile     bool
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
}

// CloseFunc flushes and closes all underlying writers.
type CloseFunc func() error

type structuredLogger struct {
	level  slog.Level
	format string
	writer io.Writer
	fields []Field
	mu     *sync.Mutex
}

// New creates a logger that writes to terminal, file, or both depending on
// Config settings. It returns a CloseFunc that should be called during shutdown
// to flush and close file writers.
func New(cfg Config) (Logger, CloseFunc, error) {
	level := parseLevel(cfg.Level)
	writers := make([]io.Writer, 0, 2)
	closers := make([]io.Closer, 0, 1)

	if cfg.ToTerminal {
		writers = append(writers, os.Stdout)
	}
	if cfg.ToFile {
		if cfg.FilePath == "" {
			return nil, nil, fmt.Errorf("log file path is required when file logging is enabled")
		}
		if err := os.MkdirAll(filepath.Dir(cfg.FilePath), 0o755); err != nil {
			return nil, nil, err
		}
		file := &lumberjack.Logger{
			Filename:   cfg.FilePath,
			MaxSize:    positiveOrDefault(cfg.MaxSizeMB, 100),
			MaxBackups: nonNegativeOrDefault(cfg.MaxBackups, 10),
			MaxAge:     nonNegativeOrDefault(cfg.MaxAgeDays, 30),
			Compress:   cfg.Compress,
			LocalTime:  true,
		}
		writers = append(writers, file)
		closers = append(closers, file)
	}
	if len(writers) == 0 {
		return nil, nil, fmt.Errorf("at least one logger writer is required")
	}

	logger := &structuredLogger{
		level:  level,
		format: strings.ToLower(strings.TrimSpace(cfg.Format)),
		writer: io.MultiWriter(writers...),
		mu:     &sync.Mutex{},
	}

	closeFn := func() error {
		var closeErr error
		for _, closer := range closers {
			if err := closer.Close(); err != nil && closeErr == nil {
				closeErr = err
			}
		}
		return closeErr
	}

	return logger, closeFn, nil
}

// WithContext stores the logger in the context under ctxkeys.Logger.
func WithContext(ctx context.Context, log Logger) context.Context {
	return context.WithValue(ctx, ctxkeys.Logger, log)
}

// FromContext retrieves the logger from context, returning a noop logger
// when none is found.
func FromContext(ctx context.Context) Logger {
	if ctx == nil {
		return noopLogger{}
	}
	if log, ok := ctx.Value(ctxkeys.Logger).(Logger); ok && log != nil {
		return log
	}
	return noopLogger{}
}

// NewNoop returns a logger that discards all output.
func NewNoop() Logger {
	return noopLogger{}
}

// String creates a string field.
func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

// Int creates an integer field.
func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

// Any creates an arbitrary field.
func Any(key string, value any) Field {
	return Field{Key: key, Value: value}
}

func (l *structuredLogger) Debug(msg string, fields ...Field) {
	l.write(slog.LevelDebug, msg, fields...)
}

func (l *structuredLogger) Info(msg string, fields ...Field) {
	l.write(slog.LevelInfo, msg, fields...)
}

func (l *structuredLogger) Warn(msg string, fields ...Field) {
	l.write(slog.LevelWarn, msg, fields...)
}

func (l *structuredLogger) Error(msg string, fields ...Field) {
	l.write(slog.LevelError, msg, fields...)
}

func (l *structuredLogger) With(fields ...Field) Logger {
	next := *l
	next.fields = append(append([]Field{}, l.fields...), fields...)
	return &next
}

func (l *structuredLogger) write(level slog.Level, msg string, fields ...Field) {
	if level < l.level {
		return
	}
	entry := map[string]any{
		"time":  time.Now().UTC().Format(time.RFC3339Nano),
		"level": strings.ToLower(level.String()),
		"msg":   msg,
	}
	for _, field := range append(append([]Field{}, l.fields...), fields...) {
		if field.Key == "" {
			continue
		}
		entry[field.Key] = redact(field.Key, field.Value)
	}
	line, err := formatEntry(l.format, entry)
	if err != nil {
		line = []byte(`{"level":"error","msg":"failed to marshal log entry"}`)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	_, err = l.writer.Write(append(line, '\n'))
	ignoreError(err)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// redact replaces sensitive field values with "[REDACTED]" to prevent leaking
// passwords, tokens, secrets, and authorization headers in log output.
func redact(key string, value any) any {
	normalized := strings.ToLower(key)
	sensitive := []string{"password", "token", "secret", "authorization", "refresh"}
	for _, marker := range sensitive {
		if strings.Contains(normalized, marker) {
			return "[REDACTED]"
		}
	}
	return value
}

func formatEntry(format string, entry map[string]any) ([]byte, error) {
	if format == "text" {
		return formatTextEntry(entry), nil
	}
	return json.Marshal(entry)
}

func formatTextEntry(entry map[string]any) []byte {
	keys := make([]string, 0, len(entry))
	for key := range entry {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(formatFieldValue(entry[key]))
	}
	return []byte(b.String())
}

func formatFieldValue(value any) string {
	switch v := value.(type) {
	case string:
		return strconv.Quote(v)
	case fmt.Stringer:
		return strconv.Quote(v.String())
	default:
		return fmt.Sprint(v)
	}
}

func positiveOrDefault(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func nonNegativeOrDefault(value int, fallback int) int {
	if value >= 0 {
		return value
	}
	return fallback
}

type noopLogger struct{}

func (noopLogger) Debug(string, ...Field) {}
func (noopLogger) Info(string, ...Field)  {}
func (noopLogger) Warn(string, ...Field)  {}
func (noopLogger) Error(string, ...Field) {}
func (noopLogger) With(...Field) Logger   { return noopLogger{} }
