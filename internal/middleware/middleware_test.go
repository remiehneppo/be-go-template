package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	router.Use(CORS(
		[]string{"https://app.example.com"},
		[]string{"GET", "POST"},
		[]string{"Authorization", "Content-Type", "X-Trace-ID", "X-Span-ID", "X-Device-ID", "X-Device-Name"},
	))
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
	if got, want := rec.Header().Get("Access-Control-Allow-Methods"), "GET,POST"; got != want {
		t.Fatalf("allow methods = %q, want %q", got, want)
	}
	if !strings.Contains(allowHeaders, "X-Device-ID") || !strings.Contains(allowHeaders, "X-Device-Name") {
		t.Fatalf("allow headers missing device fields = %q", allowHeaders)
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

func TestTimeoutReturnsGatewayTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(logger.NewNoop()))
	router.Use(Timeout(10 * time.Millisecond))
	router.GET("/slow", func(c *gin.Context) {
		<-c.Request.Context().Done()
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	req.Header.Set("X-Request-ID", "req-timeout")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"TIMEOUT"`) || !strings.Contains(rec.Body.String(), `"request_id":"req-timeout"`) {
		t.Fatalf("body = %s", rec.Body.String())
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

func TestErrorHandlerLogsWarnForClientErrorsAndErrorForServerErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := &errorCaptureLogger{}
	router := gin.New()
	router.Use(RequestID(capture))
	router.Use(ErrorHandler(capture, &fakeErrorReporter{}))
	router.GET("/client", func(c *gin.Context) {
		c.Set(string(ctxkeys.UserID), "u1")
		c.Set(string(ctxkeys.SessionID), "s1")
		ignoreError(c.Error(apperrors.New(apperrors.CodeConflict, "Conflict", http.StatusConflict)))
	})
	router.GET("/server", func(c *gin.Context) {
		c.Set(string(ctxkeys.UserID), "u1")
		c.Set(string(ctxkeys.SessionID), "s1")
		appErr := apperrors.New(apperrors.CodeInternal, "Internal server error", http.StatusInternalServerError)
		appErr.Cause = testErr("db down")
		ignoreError(c.Error(appErr))
	})

	req := httptest.NewRequest(http.MethodGet, "/client", nil)
	req.Header.Set("X-Request-ID", "req-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if capture.lastWarnMessage != "request failed" || capture.lastErrorMessage != "" {
		t.Fatalf("capture = %+v", capture)
	}
	if !capture.hasField("request_id", "req-1") || !capture.hasField("user_id", "u1") || !capture.hasField("session_id", "s1") || !capture.hasField("method", http.MethodGet) || !capture.hasField("path", "/client") || !capture.hasField("status", http.StatusConflict) || !capture.hasField("error_code", string(apperrors.CodeConflict)) || !capture.hasKey("latency_ms") {
		t.Fatalf("warn fields = %+v", capture.warnFields)
	}

	capture.reset()
	req = httptest.NewRequest(http.MethodGet, "/server", nil)
	req.Header.Set("X-Request-ID", "req-2")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if capture.lastErrorMessage != "request failed" || capture.lastWarnMessage != "" {
		t.Fatalf("capture = %+v", capture)
	}
	if !capture.hasField("request_id", "req-2") || !capture.hasField("user_id", "u1") || !capture.hasField("session_id", "s1") || !capture.hasField("status", http.StatusInternalServerError) || !capture.hasField("error_code", string(apperrors.CodeInternal)) || !capture.hasField("cause", testErr("db down")) || !capture.hasKey("latency_ms") {
		t.Fatalf("error fields = %+v", capture.errorFields)
	}
}

func TestRecoveryLogsStackAndReturnsInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := &errorCaptureLogger{}
	router := gin.New()
	router.Use(RequestID(capture))
	router.Use(Recovery(capture))
	router.GET("/panic", func(c *gin.Context) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set("X-Request-ID", "req-3")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if capture.lastErrorMessage != "panic recovered" {
		t.Fatalf("capture = %+v", capture)
	}
	if !capture.hasField("request_id", "req-3") || !capture.hasField("error_code", string(apperrors.CodeInternal)) || !capture.hasKey("stack") {
		t.Fatalf("panic fields = %+v", capture.errorFields)
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

type testErr string

func (e testErr) Error() string {
	return string(e)
}

type errorCaptureLogger struct {
	fields           []logger.Field
	warnFields       []logger.Field
	errorFields      []logger.Field
	lastWarnMessage  string
	lastErrorMessage string
}

func (l *errorCaptureLogger) Debug(string, ...logger.Field) {}
func (l *errorCaptureLogger) Info(string, ...logger.Field)  {}

func (l *errorCaptureLogger) Warn(msg string, fields ...logger.Field) {
	l.lastWarnMessage = msg
	l.warnFields = append(l.warnFields, fields...)
}

func (l *errorCaptureLogger) Error(msg string, fields ...logger.Field) {
	l.lastErrorMessage = msg
	l.errorFields = append(l.errorFields, fields...)
}

func (l *errorCaptureLogger) With(fields ...logger.Field) logger.Logger {
	l.fields = append(l.fields, fields...)
	return l
}

func (l *errorCaptureLogger) hasField(key string, want any) bool {
	for _, field := range append(append([]logger.Field{}, l.fields...), append(l.warnFields, l.errorFields...)...) {
		if field.Key == key && equalTraceFieldValue(field.Value, want) {
			return true
		}
	}
	return false
}

func (l *errorCaptureLogger) hasKey(key string) bool {
	for _, field := range append(append([]logger.Field{}, l.fields...), append(l.warnFields, l.errorFields...)...) {
		if field.Key == key {
			return true
		}
	}
	return false
}

func (l *errorCaptureLogger) reset() {
	l.warnFields = nil
	l.errorFields = nil
	l.lastWarnMessage = ""
	l.lastErrorMessage = ""
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
