package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/remihneppo/be-go-template/internal/platform/ctxkeys"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

const requestIDHeader = "X-Request-ID"
const traceIDHeader = "X-Trace-ID"

func RequestID(log logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := sanitizeID(c.GetHeader(requestIDHeader))
		if requestID == "" {
			requestID = newID()
		}
		traceID := sanitizeID(c.GetHeader(traceIDHeader))
		if traceID == "" {
			traceID = requestID
		}

		ctx := context.WithValue(c.Request.Context(), ctxkeys.RequestID, requestID)
		ctx = context.WithValue(ctx, ctxkeys.TraceID, traceID)
		ctx = logger.WithContext(ctx, log.With(logger.String("request_id", requestID), logger.String("trace_id", traceID)))
		c.Request = c.Request.WithContext(ctx)

		c.Set(string(ctxkeys.RequestID), requestID)
		c.Set(string(ctxkeys.TraceID), traceID)
		c.Header(requestIDHeader, requestID)
		c.Next()
	}
}

func sanitizeID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r == '.' || r == ':' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return ""
		}
	}
	return value
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "request-id-unavailable"
	}
	return hex.EncodeToString(b[:])
}

func WriteRequestIDHeader(w http.ResponseWriter, requestID string) {
	if requestID != "" {
		w.Header().Set(requestIDHeader, requestID)
	}
}
