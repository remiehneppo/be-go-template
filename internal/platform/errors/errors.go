package apperrors

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
)

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
		Stack:       debug.Stack(),
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
	return Wrap("", err, CodeInternal, "Internal server error", http.StatusInternalServerError)
}
