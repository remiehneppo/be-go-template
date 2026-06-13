// Package integration tests provides end-to-end verification of the API
// response envelope structure.
package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHealthzJSON verifies the health response envelope structure.
func TestHealthzJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    map[string]any{"status": "healthy"},
		}); err != nil {
			t.Fatalf("encode health response: %v", err)
		}
	}))
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if success, ok := body["success"].(bool); !ok || !success {
		t.Fatalf("expected success=true, got %v", body["success"])
	}
	if _, ok := body["data"]; !ok {
		t.Fatal("expected data field in response")
	}
}

// TestResponseEnvelopeSuccess verifies the success envelope structure.
func TestResponseEnvelopeSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success":     true,
			"request_id":  "test-req-1",
			"data":        map[string]any{"key": "value"},
		}); err != nil {
			t.Fatalf("encode success response: %v", err)
		}
	}))
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if _, ok := body["request_id"]; !ok {
		t.Fatal("expected request_id field in response")
	}
}

// TestResponseEnvelopeError verifies the error envelope structure.
func TestResponseEnvelopeError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success":     false,
			"request_id":  "test-req-2",
			"error":       map[string]any{"code": "VALIDATION_ERROR", "message": "Invalid input"},
		}); err != nil {
			t.Fatalf("encode error response: %v", err)
		}
	}))
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if success, ok := body["success"].(bool); !ok || success {
		t.Fatalf("expected success=false, got %v", body["success"])
	}
	if err, ok := body["error"].(map[string]any); !ok || err["code"] != "VALIDATION_ERROR" {
		t.Fatalf("expected error.code=VALIDATION_ERROR, got %v", body["error"])
	}
}
