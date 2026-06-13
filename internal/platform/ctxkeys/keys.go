// Package ctxkeys defines stable context keys used across middleware, services,
// and logging to avoid magic strings and reduce key collisions.
package ctxkeys

// Key is a typed string representing a request-scoped context key.
type Key string

const (
	// RequestID carries the per-request identifier propagated through logs.
	RequestID Key = "request_id"
	// UserID carries the authenticated user identifier when available.
	UserID Key = "user_id"
	// SessionID carries the current session identifier.
	SessionID Key = "session_id"
	// TokenID carries the current access token identifier (jwt jti).
	TokenID Key = "token_id"
	// Roles carries the roles associated with the current token.
	Roles Key = "roles"
	// TraceID carries an optional distributed trace identifier.
	TraceID Key = "trace_id"
	// SpanID carries an optional span identifier within a trace.
	SpanID Key = "span_id"
	// Logger carries the structured logger instance for the request.
	Logger Key = "logger"
	// RequestStartedAt carries the time the request was first received.
	RequestStartedAt Key = "request_started_at"
)
