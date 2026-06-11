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
