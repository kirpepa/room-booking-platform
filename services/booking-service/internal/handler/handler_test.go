package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestExtractAuth_Valid(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	uid := uuid.New()
	r.Header.Set("X-User-ID", uid.String())
	r.Header.Set("X-User-Role", "user")

	gotUID, gotRole := extractAuth(r)
	if gotUID != uid {
		t.Errorf("expected %s, got %s", uid, gotUID)
	}
	if gotRole != "user" {
		t.Errorf("expected user, got %s", gotRole)
	}
}

func TestExtractAuth_Missing(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)

	gotUID, gotRole := extractAuth(r)
	if gotUID != uuid.Nil {
		t.Errorf("expected nil UUID, got %s", gotUID)
	}
	if gotRole != "" {
		t.Errorf("expected empty role, got %s", gotRole)
	}
}

func TestExtractAuth_InvalidUUID(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-User-ID", "not-a-uuid")
	r.Header.Set("X-User-Role", "admin")

	gotUID, _ := extractAuth(r)
	if gotUID != uuid.Nil {
		t.Errorf("expected nil UUID for invalid input, got %s", gotUID)
	}
}

func TestQueryInt(t *testing.T) {
	tests := []struct {
		query    string
		key      string
		def      int
		expected int
	}{
		{"?page=5", "page", 1, 5},
		{"?page=abc", "page", 1, 1},
		{"", "page", 1, 1},
		{"?pageSize=50", "pageSize", 20, 50},
	}

	for _, tt := range tests {
		t.Run(tt.query+"_"+tt.key, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/test"+tt.query, nil)
			got := queryInt(r, tt.key, tt.def)
			if got != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, got)
			}
		})
	}
}

func TestCreateBooking_AdminForbidden(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/bookings/create", nil)
	req.Header.Set("X-User-ID", uuid.New().String())
	req.Header.Set("X-User-Role", "admin")

	h.CreateBooking(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestCreateBooking_NoAuth(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/bookings/create", nil)

	h.CreateBooking(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestMyBookings_AdminForbidden(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/bookings/my", nil)
	req.Header.Set("X-User-ID", uuid.New().String())
	req.Header.Set("X-User-Role", "admin")

	h.MyBookings(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestListBookings_UserForbidden(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/bookings/list", nil)
	req.Header.Set("X-User-ID", uuid.New().String())
	req.Header.Set("X-User-Role", "user")

	h.ListBookings(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestCancelBooking_NoAuth(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/bookings/123/cancel", nil)

	h.CancelBooking(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}
