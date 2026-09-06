package handler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/room-booking/services/auth-service/internal/service"
)

type stubAuthService struct {
	registerCalls int
	dummyCalls    int
	seedCalls     int
}

func (s *stubAuthService) Register(_ context.Context, email, _ string, role string) (*service.UserResponse, error) {
	s.registerCalls++
	return &service.UserResponse{ID: uuid.New(), Email: email, Role: role}, nil
}

func (s *stubAuthService) Login(_ context.Context, _, _ string) (string, error) {
	return "token", nil
}

func (s *stubAuthService) DummyLogin(_ context.Context, _ string) (string, error) {
	s.dummyCalls++
	return "test-token", nil
}

func (s *stubAuthService) Seed(_ context.Context) error {
	s.seedCalls++
	return nil
}

func testHandler(svc authAPI, testMode bool) http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(svc, log, testMode).Routes()
}

func request(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	return rec
}

func TestProductionRoutesDisableTestBackdoors(t *testing.T) {
	svc := &stubAuthService{}
	h := testHandler(svc, false)

	if got := request(t, h, "/dummyLogin", `{"role":"admin"}`).Code; got != http.StatusNotFound {
		t.Fatalf("dummyLogin must be absent outside TEST_TASK_MODE, got %d", got)
	}
	if got := request(t, h, "/seed", `{}`).Code; got != http.StatusNotFound {
		t.Fatalf("seed must be absent outside TEST_TASK_MODE, got %d", got)
	}
	if svc.dummyCalls != 0 || svc.seedCalls != 0 {
		t.Fatal("disabled test endpoints reached the service")
	}
}

func TestProductionRegistrationCannotCreateAdmin(t *testing.T) {
	svc := &stubAuthService{}
	rec := request(t, testHandler(svc, false), "/register",
		`{"email":"admin@example.com","password":"password123","role":"admin"}`)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.registerCalls != 0 {
		t.Fatal("admin self-registration reached the service")
	}
}

func TestProductionRegistrationAllowsUser(t *testing.T) {
	svc := &stubAuthService{}
	rec := request(t, testHandler(svc, false), "/register",
		`{"email":"user@example.com","password":"password123","role":"user"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.registerCalls != 1 {
		t.Fatalf("expected one registration call, got %d", svc.registerCalls)
	}
}

func TestTestModeEnablesDummyLogin(t *testing.T) {
	svc := &stubAuthService{}
	rec := request(t, testHandler(svc, true), "/dummyLogin", `{"role":"admin"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.dummyCalls != 1 {
		t.Fatalf("expected one dummy login call, got %d", svc.dummyCalls)
	}
}

func TestRequestRejectsUnknownFieldsAndMultipleDocuments(t *testing.T) {
	svc := &stubAuthService{}
	h := testHandler(svc, false)

	for _, body := range []string{
		`{"email":"user@example.com","password":"password123","role":"user","isAdmin":true}`,
		`{"email":"user@example.com","password":"password123","role":"user"} {}`,
	} {
		rec := request(t, h, "/register", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %q, got %d", body, rec.Code)
		}
	}
	if svc.registerCalls != 0 {
		t.Fatal("invalid payload reached the service")
	}
}
