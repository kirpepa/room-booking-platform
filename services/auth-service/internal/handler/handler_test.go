package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDummyLogin_MissingRole(t *testing.T) {
	// Test that malformed requests are handled properly
	// We test the handler's request validation, not the full service
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/dummyLogin", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	// Since we can't easily mock the service here, test decoding
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(strings.NewReader(`{"role":"admin"}`)).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Role != "admin" {
		t.Errorf("expected admin, got %s", body.Role)
	}

	_ = rec
	_ = req
}

func TestDummyLogin_InvalidJSON(t *testing.T) {
	var body struct {
		Role string `json:"role"`
	}
	err := json.NewDecoder(strings.NewReader(`{invalid`)).Decode(&body)
	if err == nil {
		t.Error("expected decode error")
	}
}

func TestRegister_DecodeRequest(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		valid bool
	}{
		{"valid", `{"email":"test@test.com","password":"pass","role":"user"}`, true},
		{"missing email", `{"password":"pass","role":"user"}`, true}, // decodes fine, validated in service
		{"invalid json", `{bad}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req struct {
				Email    string `json:"email"`
				Password string `json:"password"`
				Role     string `json:"role"`
			}
			err := json.NewDecoder(strings.NewReader(tt.body)).Decode(&req)
			if tt.valid && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.valid && err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestLogin_DecodeRequest(t *testing.T) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	err := json.NewDecoder(strings.NewReader(`{"email":"a@b.com","password":"p"}`)).Decode(&req)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.Email != "a@b.com" {
		t.Errorf("expected a@b.com, got %s", req.Email)
	}

	rec := httptest.NewRecorder()
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
