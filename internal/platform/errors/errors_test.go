package apperrors

import (
	"errors"
	"net/http"
	"testing"
)

func TestFromErrorPreservesAppError(t *testing.T) {
	want := New(CodeNotFound, "missing", http.StatusNotFound)

	got := FromError(want)
	if got != want {
		t.Fatal("FromError() did not preserve AppError")
	}
}

func TestFromErrorWrapsUnknownError(t *testing.T) {
	got := FromError(errors.New("database password leaked here"))

	if got.Code != CodeInternal {
		t.Fatalf("Code = %s", got.Code)
	}
	if got.SafeMessage != "Internal server error" {
		t.Fatalf("SafeMessage = %q", got.SafeMessage)
	}
}

func TestDependencyWrapsAsRetryableServiceUnavailable(t *testing.T) {
	inner := errors.New("timeout")
	got := Dependency("MongoDatabase.FindOne", inner)

	if got.Code != CodeDependency {
		t.Fatalf("Code = %s", got.Code)
	}
	if got.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("HTTPStatus = %d", got.HTTPStatus)
	}
	if !got.Retryable {
		t.Fatal("Retryable = false")
	}
	if got.Unwrap() != inner {
		t.Fatalf("Cause = %v", got.Unwrap())
	}
}

func TestFromErrorMapsDomainSentinels(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code Code
	}{
		{name: "not_found", err: ErrNotFound, code: CodeNotFound},
		{name: "unauthorized", err: ErrUnauthorized, code: CodeUnauthorized},
		{name: "forbidden", err: ErrForbidden, code: CodeForbidden},
		{name: "conflict", err: ErrConflict, code: CodeConflict},
		{name: "validation", err: ErrValidation, code: CodeValidation},
		{name: "token_expired", err: ErrTokenExpired, code: CodeTokenExpired},
		{name: "token_revoked", err: ErrTokenRevoked, code: CodeTokenRevoked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FromError(tc.err)
			if got.Code != tc.code {
				t.Fatalf("Code = %s", got.Code)
			}
			if got.HTTPStatus != StatusForCode(tc.code) {
				t.Fatalf("HTTPStatus = %d", got.HTTPStatus)
			}
		})
	}
}

func TestTokenErrorConstructorsWrapSentinels(t *testing.T) {
	expired := TokenExpired("expired")
	if !errors.Is(expired, ErrTokenExpired) {
		t.Fatal("TokenExpired() does not wrap ErrTokenExpired")
	}
	revoked := TokenRevoked("revoked")
	if !errors.Is(revoked, ErrTokenRevoked) {
		t.Fatal("TokenRevoked() does not wrap ErrTokenRevoked")
	}
}

func TestWrapHonorsStackToggle(t *testing.T) {
	SetStackTraceEnabled(false)
	t.Cleanup(func() { SetStackTraceEnabled(true) })

	got := Wrap("op", errors.New("boom"), CodeInternal, "safe", http.StatusInternalServerError)
	if got.Stack != nil {
		t.Fatalf("Stack = %q", string(got.Stack))
	}
}

func TestWrapCapturesStackWhenEnabled(t *testing.T) {
	SetStackTraceEnabled(true)

	got := Wrap("op", errors.New("boom"), CodeInternal, "safe", http.StatusInternalServerError)
	if len(got.Stack) == 0 {
		t.Fatal("Stack = empty")
	}
}
