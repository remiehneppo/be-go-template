package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/platform/ctxkeys"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

func TestRequestIDUsesIncomingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(logger.NewNoop()))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true, "request_id": c.GetString("request_id"), "data": gin.H{"ok": true}})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", "req-123")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "req-123" {
		t.Fatalf("X-Request-ID = %q", got)
	}
	if !strings.Contains(rec.Body.String(), `"request_id":"req-123"`) {
		t.Fatalf("body missing request id: %s", rec.Body.String())
	}
}

func TestRequestIDUsesIncomingTraceHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := &traceCaptureLogger{}
	router := gin.New()
	router.Use(RequestID(capture))
	router.GET("/ping", func(c *gin.Context) {
		if got, _ := c.Request.Context().Value(ctxkeys.TraceID).(string); got != "trace-123" {
			t.Fatalf("context trace id = %q", got)
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "request_id": c.GetString("request_id"), "data": gin.H{"ok": true}})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", "req-123")
	req.Header.Set("X-Trace-ID", "trace-123")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "req-123" {
		t.Fatalf("X-Request-ID = %q", got)
	}
	if !capture.hasField("request_id", "req-123") || !capture.hasField("trace_id", "trace-123") {
		t.Fatalf("logger fields = %+v", capture.fields)
	}
}

func TestRequestIDUsesIncomingSpanHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := &traceCaptureLogger{}
	router := gin.New()
	router.Use(RequestID(capture))
	router.GET("/ping", func(c *gin.Context) {
		if got, _ := c.Request.Context().Value(ctxkeys.SpanID).(string); got != "span-123" {
			t.Fatalf("context span id = %q", got)
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", "req-123")
	req.Header.Set("X-Span-ID", "span-123")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !capture.hasField("span_id", "span-123") {
		t.Fatalf("logger fields = %+v", capture.fields)
	}
}

func TestRequestIDFallsBackToRequestIDForInvalidTraceHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := &traceCaptureLogger{}
	router := gin.New()
	router.Use(RequestID(capture))
	router.GET("/ping", func(c *gin.Context) {
		if got, _ := c.Request.Context().Value(ctxkeys.TraceID).(string); got != "req-123" {
			t.Fatalf("context trace id = %q", got)
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", "req-123")
	req.Header.Set("X-Trace-ID", "invalid trace id!")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !capture.hasField("trace_id", "req-123") {
		t.Fatalf("logger fields = %+v", capture.fields)
	}
}

func TestCORSAllowsTraceAndSpanHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS([]string{"https://app.example.com"}))
	router.OPTIONS("/ping", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	allowHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowHeaders, "X-Trace-ID") || !strings.Contains(allowHeaders, "X-Span-ID") {
		t.Fatalf("allow headers = %q", allowHeaders)
	}
}

func TestBodyLimitRejectsLargeBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(logger.NewNoop()))
	router.Use(BodyLimit(4))
	router.POST("/data", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true, "request_id": c.GetString("request_id"), "data": gin.H{"ok": true}})
	})

	req := httptest.NewRequest(http.MethodPost, "/data", strings.NewReader("too-large"))
	req.ContentLength = int64(len("too-large"))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestErrorHandlerAppendsErrorEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reporter := &fakeErrorReporter{}
	router := gin.New()
	router.Use(RequestID(logger.NewNoop()))
	router.Use(ErrorHandler(logger.NewNoop(), reporter))
	router.GET("/boom", func(c *gin.Context) {
		ignoreError(c.Error(apperrors.New(apperrors.CodeConflict, "Conflict", http.StatusConflict)))
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	req.Header.Set("X-Request-ID", "req-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if len(reporter.events) != 1 {
		t.Fatalf("events = %+v", reporter.events)
	}
	event := reporter.events[0]
	if event.RequestID != "req-1" || event.ErrorCode != string(apperrors.CodeConflict) || event.Path != "/boom" || event.Method != http.MethodGet {
		t.Fatalf("event = %+v", event)
	}
}

type fakeErrorReporter struct {
	events []auth.ErrorEvent
}

func (r *fakeErrorReporter) Append(ctx context.Context, event auth.ErrorEvent) error {
	r.events = append(r.events, event)
	return nil
}

func ignoreError(err error) {
	_ = err
}

type traceCaptureLogger struct {
	fields []logger.Field
}

func (l *traceCaptureLogger) Debug(string, ...logger.Field) {}
func (l *traceCaptureLogger) Info(string, ...logger.Field)  {}
func (l *traceCaptureLogger) Warn(string, ...logger.Field)  {}
func (l *traceCaptureLogger) Error(string, ...logger.Field) {}

func (l *traceCaptureLogger) With(fields ...logger.Field) logger.Logger {
	l.fields = append(l.fields, fields...)
	return l
}

func (l *traceCaptureLogger) hasField(key string, want any) bool {
	for _, field := range l.fields {
		if field.Key == key && equalTraceFieldValue(field.Value, want) {
			return true
		}
	}
	return false
}

func equalTraceFieldValue(got any, want any) bool {
	switch wantValue := want.(type) {
	case []string:
		gotValue, ok := got.([]string)
		if !ok || len(gotValue) != len(wantValue) {
			return false
		}
		for i := range gotValue {
			if gotValue[i] != wantValue[i] {
				return false
			}
		}
		return true
	default:
		return got == want
	}
}
