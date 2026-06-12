package ctxkeys

type Key string

const (
	RequestID        Key = "request_id"
	UserID           Key = "user_id"
	SessionID        Key = "session_id"
	TokenID          Key = "token_id"
	Roles            Key = "roles"
	TraceID          Key = "trace_id"
	SpanID           Key = "span_id"
	Logger           Key = "logger"
	RequestStartedAt Key = "request_started_at"
)
