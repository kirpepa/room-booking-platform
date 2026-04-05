package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	pkgjwt "github.com/room-booking/pkg/jwt"
)

func TestRequireAuth_NoToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	m := NewAuthMiddleware(&key.PublicKey)

	handler := m.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuth_ValidToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	m := NewAuthMiddleware(&key.PublicKey)

	userID := uuid.New()
	token, _ := pkgjwt.GenerateToken(key, userID, "admin", time.Hour)

	var gotUserID, gotRole string
	handler := m.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = r.Header.Get("X-User-ID")
		gotRole = r.Header.Get("X-User-Role")
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if gotUserID != userID.String() {
		t.Errorf("expected user id %s, got %s", userID, gotUserID)
	}
	if gotRole != "admin" {
		t.Errorf("expected admin, got %s", gotRole)
	}
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	m := NewAuthMiddleware(&key.PublicKey)

	handler := m.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireRole_Allowed(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	m := NewAuthMiddleware(&key.PublicKey)

	handler := m.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-User-Role", "admin")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRequireRole_MultipleAllowed(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	m := NewAuthMiddleware(&key.PublicKey)

	handler := m.RequireRole("admin", "user")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-User-Role", "user")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRequireRole_Forbidden(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	m := NewAuthMiddleware(&key.PublicKey)

	handler := m.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-User-Role", "user")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		header   string
		expected string
	}{
		{"Bearer abc123", "abc123"},
		{"bearer abc123", "abc123"},
		{"abc123", ""},
		{"", ""},
		{"Basic abc123", ""},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			got := extractBearerToken(r)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}
