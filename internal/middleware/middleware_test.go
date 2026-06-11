package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/domain/auth"
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
