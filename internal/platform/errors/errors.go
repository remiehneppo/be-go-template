package apperrors

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"sync/atomic"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
	ErrValidation   = errors.New("validation failed")
	ErrTokenRevoked = errors.New("token revoked")
	ErrTokenExpired = errors.New("token expired")
)

var stackTraceEnabled atomic.Bool

func init() {
	stackTraceEnabled.Store(true)
}

func SetStackTraceEnabled(enabled bool) {
	stackTraceEnabled.Store(enabled)
}

type ValidationDetail struct {
	Field  string         `json:"field"`
	Reason string         `json:"reason"`
	Meta   map[string]any `json:"meta,omitempty"`
}

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

func New(code Code, safeMessage string, status int) *AppError {
	return &AppError{
		Code:        code,
		Message:     safeMessage,
		SafeMessage: safeMessage,
		HTTPStatus:  status,
	}
}

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

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

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
