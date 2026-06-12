package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	domainauth "github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/platform/ctxkeys"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

func TestLoggingMiddlewareUsesContextLoggerFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := &accessLogCaptureLogger{}
	tokens := &fakeTokenService{claims: &domainauth.AccessClaims{
		UserID:    "u1",
		SessionID: "s1",
		TokenID:   "jti1",
		Roles:     []string{"user"},
	}}

	router := gin.New()
	router.Use(RequestID(capture))
	router.Use(Authenticate(tokens, nil))
	router.Use(Logging(logger.NewNoop()))
	router.GET("/me", func(c *gin.Context) {
		if c.GetString(string(ctxkeys.UserID)) != "u1" {
			t.Fatalf("gin user id = %q", c.GetString(string(ctxkeys.UserID)))
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/me?debug=true", nil)
	req.Header.Set("X-Request-ID", "req-123")
	req.Header.Set("X-Trace-ID", "trace-123")
	req.Header.Set("Authorization", "Bearer access-token")
	req.Header.Set("User-Agent", "be-go-template-test/1.0")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if capture.lastMessage != "http request" {
		t.Fatalf("message = %q", capture.lastMessage)
	}
	if !capture.hasField("request_id", "req-123") || !capture.hasField("trace_id", "trace-123") || !capture.hasField("user_id", "u1") || !capture.hasField("session_id", "s1") || !capture.hasField("token_id", "jti1") {
		t.Fatalf("logger fields = %+v", capture.fields)
	}
	if !capture.hasField("method", http.MethodGet) || !capture.hasField("path", "/me") || !capture.hasField("query", "debug=true") || !capture.hasField("status", http.StatusNoContent) || !capture.hasKey("ip") || !capture.hasField("user_agent", "be-go-template-test/1.0") {
		t.Fatalf("access log fields = %+v", capture.infoFields)
	}
	if !capture.hasKey("latency_ms") {
		t.Fatalf("latency fields = %+v", capture.infoFields)
	}
}

type accessLogCaptureLogger struct {
	fields      []logger.Field
	infoFields  []logger.Field
	lastMessage string
}

func (l *accessLogCaptureLogger) Debug(string, ...logger.Field) {}
func (l *accessLogCaptureLogger) Warn(string, ...logger.Field)  {}
func (l *accessLogCaptureLogger) Error(string, ...logger.Field) {}

func (l *accessLogCaptureLogger) Info(msg string, fields ...logger.Field) {
	l.lastMessage = msg
	l.infoFields = append(l.infoFields, fields...)
}

func (l *accessLogCaptureLogger) With(fields ...logger.Field) logger.Logger {
	l.fields = append(l.fields, fields...)
	return l
}

func (l *accessLogCaptureLogger) hasField(key string, want any) bool {
	for _, field := range append(append([]logger.Field{}, l.fields...), l.infoFields...) {
		if field.Key == key && equalAccessFieldValue(field.Value, want) {
			return true
		}
	}
	return false
}

func (l *accessLogCaptureLogger) hasKey(key string) bool {
	for _, field := range append(append([]logger.Field{}, l.fields...), l.infoFields...) {
		if field.Key == key {
			return true
		}
	}
	return false
}

func equalAccessFieldValue(got any, want any) bool {
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
	case int:
		gotValue, ok := got.(int)
		return ok && gotValue == wantValue
	case int64:
		gotValue, ok := got.(int64)
		return ok && gotValue == wantValue
	default:
		return got == want
	}
}
