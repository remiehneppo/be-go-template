package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/remihneppo/be-go-template/internal/platform/ctxkeys"
)

type Field struct {
	Key   string
	Value any
}

type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	With(fields ...Field) Logger
}

type Config struct {
	Level      string
	Format     string
	FilePath   string
	ToTerminal bool
	ToFile     bool
}

type CloseFunc func() error

type structuredLogger struct {
	level  slog.Level
	writer io.Writer
	fields []Field
	mu     *sync.Mutex
}

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
		file, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, err
		}
		writers = append(writers, file)
		closers = append(closers, file)
	}
	if len(writers) == 0 {
		return nil, nil, fmt.Errorf("at least one logger writer is required")
	}

	logger := &structuredLogger{
		level:  level,
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

func WithContext(ctx context.Context, log Logger) context.Context {
	return context.WithValue(ctx, ctxkeys.Logger, log)
}

func FromContext(ctx context.Context) Logger {
	if ctx == nil {
		return noopLogger{}
	}
	if log, ok := ctx.Value(ctxkeys.Logger).(Logger); ok && log != nil {
		return log
	}
	return noopLogger{}
}

func NewNoop() Logger {
	return noopLogger{}
}

func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

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
	line, err := json.Marshal(entry)
	if err != nil {
		line = []byte(`{"level":"error","msg":"failed to marshal log entry"}`)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.writer.Write(append(line, '\n'))
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

type noopLogger struct{}

func (noopLogger) Debug(string, ...Field) {}
func (noopLogger) Info(string, ...Field)  {}
func (noopLogger) Warn(string, ...Field)  {}
func (noopLogger) Error(string, ...Field) {}
func (noopLogger) With(...Field) Logger   { return noopLogger{} }
