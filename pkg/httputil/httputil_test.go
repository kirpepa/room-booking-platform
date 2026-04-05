package httputil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSON(t *testing.T) {
	w := httptest.NewRecorder()
	JSON(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
	if !strings.Contains(w.Body.String(), `"key":"value"`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestError(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, http.StatusBadRequest, "INVALID_REQUEST", "bad request")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "INVALID_REQUEST") {
		t.Errorf("expected error code in body: %s", body)
	}
}

func TestBadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	BadRequest(w, "test message")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestUnauthorized(t *testing.T) {
	w := httptest.NewRecorder()
	Unauthorized(w)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestForbidden(t *testing.T) {
	w := httptest.NewRecorder()
	Forbidden(w, "no access")
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	NotFound(w, "NOT_FOUND", "not found")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestConflict(t *testing.T) {
	w := httptest.NewRecorder()
	Conflict(w, "CONFLICT", "conflict")
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestInternalError(t *testing.T) {
	w := httptest.NewRecorder()
	InternalError(w)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestDecodeJSON(t *testing.T) {
	body := strings.NewReader(`{"name":"test"}`)
	r := httptest.NewRequest("POST", "/", body)

	var result struct {
		Name string `json:"name"`
	}
	if err := DecodeJSON(r, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Name != "test" {
		t.Errorf("expected test, got %s", result.Name)
	}
}

func TestDecodeJSON_Invalid(t *testing.T) {
	body := strings.NewReader(`{invalid}`)
	r := httptest.NewRequest("POST", "/", body)

	var result struct{}
	if err := DecodeJSON(r, &result); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
