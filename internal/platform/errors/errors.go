// Package apperrors provides a structured application error type that wraps
// internal causes while exposing safe messages to clients.
//
// Every HTTP handler returns *AppError so that the error middleware can map
// to a stable JSON envelope with HTTP status, request id, and optional
// field-level validation details.
package apperrors

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"sync/atomic"
)

var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = errors.New("not found")
	// ErrUnauthorized is returned when authentication credentials are missing or invalid.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden is returned when the authenticated user lacks the required role or permission.
	ErrForbidden = errors.New("forbidden")
	// ErrConflict is returned when a requested operation violates a uniqueness constraint.
	ErrConflict = errors.New("conflict")
	// ErrValidation is returned when request input fails validation.
	ErrValidation = errors.New("validation failed")
	// ErrTokenRevoked is returned when a token has been explicitly revoked.
	ErrTokenRevoked = errors.New("token revoked")
	// ErrTokenExpired is returned when a token has expired.
	ErrTokenExpired = errors.New("token expired")
)

var stackTraceEnabled atomic.Bool

func init() {
	stackTraceEnabled.Store(true)
}

// SetStackTraceEnabled controls whether internal stack traces are captured
// and logged. It should be set to false in production to avoid leaking
// implementation details.
func SetStackTraceEnabled(enabled bool) {
	stackTraceEnabled.Store(enabled)
}

// ValidationDetail describes a single field-level validation failure.
type ValidationDetail struct {
	Field  string         `json:"field"`
	Reason string         `json:"reason"`
	Meta   map[string]any `json:"meta,omitempty"`
}

// AppError is a structured error that carries an error code, safe message,
// HTTP status, optional validation details, and an internal cause.
//
// The SafeMessage field is safe for client exposure. The Message field may
// contain internal details visible only in server logs.
type AppError struct {
	Code        Code               `json:"code"`
	Message     string             `json:"-"`
	SafeMessage string             `json:"message"`
	HTTPStatus  int                `json:"-"`
	Cause       error              `json:"-"`
	Details     []ValidationDetail `json:"details,omitempty"`
	Op          string             `json:"-"`
	Stack       []byte             `json:"-"`
	Retryable   bool               `json:"-"`
}

// New creates a new AppError with the given error code, safe message, and
// HTTP status.
func New(code Code, safeMessage string, status int) *AppError {
	return &AppError{
		Code:        code,
		Message:     safeMessage,
		SafeMessage: safeMessage,
		HTTPStatus:  status,
	}
}

// WithOp adds an operation name to the error for structured logging.
func (e *AppError) WithOp(op string) *AppError {
	if e == nil {
		return nil
	}
	next := *e
	next.Op = op
	return &next
}

// Wrap creates a new AppError that wraps an existing error with operation
// context and stack capture.
func Wrap(op string, err error, code Code, safeMessage string, status int) *AppError {
	if err == nil {
		return New(code, safeMessage, status)
	}
	return &AppError{
		Code:        code,
		Message:     err.Error(),
		SafeMessage: safeMessage,
		HTTPStatus:  status,
		Cause:       err,
		Op:          op,
		Stack:       captureStack(),
	}
}

// Dependency creates an AppError indicating a downstream dependency failure.
// The error is marked retryable and uses HTTP 503.
func Dependency(op string, err error) *AppError {
	if err == nil {
		return nil
	}
	return &AppError{
		Code:        CodeDependency,
		Message:     err.Error(),
		SafeMessage: "Dependency unavailable",
		HTTPStatus:  http.StatusServiceUnavailable,
		Cause:       err,
		Op:          op,
		Retryable:   true,
	}
}

// Validation creates a validation error with optional field-level details.
func Validation(message string, details []ValidationDetail) *AppError {
	return &AppError{
		Code:        CodeValidation,
		Message:     message,
		SafeMessage: message,
		HTTPStatus:  http.StatusBadRequest,
		Details:     details,
	}
}

func captureStack() []byte {
	if !stackTraceEnabled.Load() {
		return nil
	}
	return debug.Stack()
}

// TokenExpired creates an AppError for an expired token.
func TokenExpired(message string) *AppError {
	if message == "" {
		message = "Token expired"
	}
	return &AppError{
		Code:        CodeTokenExpired,
		Message:     message,
		SafeMessage: "Unauthorized",
		HTTPStatus:  http.StatusUnauthorized,
		Cause:       ErrTokenExpired,
	}
}

// TokenRevoked creates an AppError for a revoked token.
func TokenRevoked(message string) *AppError {
	if message == "" {
		message = "Token revoked"
	}
	return &AppError{
		Code:        CodeTokenRevoked,
		Message:     message,
		SafeMessage: "Unauthorized",
		HTTPStatus:  http.StatusUnauthorized,
		Cause:       ErrTokenRevoked,
	}
}

// Error returns a human-readable representation including the operation name
// and message when available.
func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Op != "" && e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Op, e.Message)
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Code)
}

// Unwrap returns the internal cause error, if any.
func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// FromError extracts an *AppError from a generic error. When the error is
// already an *AppError it is returned directly; otherwise it is mapped to
// a suitable AppError based on well-known sentinel errors.
func FromError(err error) *AppError {
	if err == nil {
		return nil
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return New(CodeNotFound, "Not found", http.StatusNotFound)
	case errors.Is(err, ErrUnauthorized):
		return New(CodeUnauthorized, "Unauthorized", http.StatusUnauthorized)
	case errors.Is(err, ErrForbidden):
		return New(CodeForbidden, "Forbidden", http.StatusForbidden)
	case errors.Is(err, ErrConflict):
		return New(CodeConflict, "Conflict", http.StatusConflict)
	case errors.Is(err, ErrValidation):
		return Validation("Validation failed", nil)
	case errors.Is(err, ErrTokenExpired):
		return TokenExpired(err.Error())
	case errors.Is(err, ErrTokenRevoked):
		return TokenRevoked(err.Error())
	}
	return Wrap("", err, CodeInternal, "Internal server error", http.StatusInternalServerError)
}
