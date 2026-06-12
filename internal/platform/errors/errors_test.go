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
