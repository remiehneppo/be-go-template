package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
)

func TestErrorResponseUsesSafeMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	Error(c, apperrors.Wrap("op", assertErr("secret cause"), apperrors.CodeInternal, "safe message", http.StatusInternalServerError))

	body := rec.Body.String()
	if strings.Contains(body, "secret cause") {
		t.Fatalf("response leaked cause: %s", body)
	}
	if !strings.Contains(body, "safe message") {
		t.Fatalf("response missing safe message: %s", body)
	}
}

func TestErrorResponseIncludesRetryableForDependencyErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	Error(c, apperrors.Dependency("MongoDatabase.FindOne", assertErr("timeout")))

	var response struct {
		Error struct {
			Retryable bool `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v body = %s", err, rec.Body.String())
	}
	if !response.Error.Retryable {
		t.Fatalf("retryable = false body = %s", rec.Body.String())
	}
}

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}
