package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/room-booking/services/booking-service/internal/service"
)

type stubBookingAPI struct {
	createRes *service.BookingResponse
	createErr error
	cancelRes *service.BookingResponse
	cancelErr error
	my        []service.BookingResponse
	myErr     error
	list      []service.BookingResponse
	listTotal int
	listErr   error
}

func (s *stubBookingAPI) CreateBooking(ctx context.Context, userID uuid.UUID, role string, req service.CreateBookingRequest) (*service.BookingResponse, error) {
	return s.createRes, s.createErr
}

func (s *stubBookingAPI) CancelBooking(ctx context.Context, bookingID, userID uuid.UUID) (*service.BookingResponse, error) {
	return s.cancelRes, s.cancelErr
}

func (s *stubBookingAPI) GetMyBookings(ctx context.Context, userID uuid.UUID) ([]service.BookingResponse, error) {
	return s.my, s.myErr
}

func (s *stubBookingAPI) ListBookings(ctx context.Context, page, pageSize int) ([]service.BookingResponse, int, error) {
	return s.list, s.listTotal, s.listErr
}

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCreateBooking_HandlerBranches(t *testing.T) {
	uid := uuid.New()
	slotID := uuid.New()

	t.Run("success", func(t *testing.T) {
		st := &stubBookingAPI{
			createRes: &service.BookingResponse{ID: uuid.New(), SlotID: slotID, UserID: uid, Status: "active"},
		}
		h := New(st, discardLog())
		body := map[string]interface{}{"slotId": slotID.String()}
		buf, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/bookings/create", bytes.NewReader(buf))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-ID", uid.String())
		req.Header.Set("X-User-Role", "user")
		rec := httptest.NewRecorder()
		h.CreateBooking(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("slot_not_found", func(t *testing.T) {
		st := &stubBookingAPI{createErr: service.ErrSlotNotFound}
		h := New(st, discardLog())
		buf, _ := json.Marshal(map[string]string{"slotId": slotID.String()})
		req := httptest.NewRequest("POST", "/bookings/create", bytes.NewReader(buf))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-ID", uid.String())
		req.Header.Set("X-User-Role", "user")
		rec := httptest.NewRecorder()
		h.CreateBooking(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("slot_in_past", func(t *testing.T) {
		st := &stubBookingAPI{createErr: service.ErrSlotInPast}
		h := New(st, discardLog())
		buf, _ := json.Marshal(map[string]string{"slotId": slotID.String()})
		req := httptest.NewRequest("POST", "/bookings/create", bytes.NewReader(buf))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-ID", uid.String())
		req.Header.Set("X-User-Role", "user")
		rec := httptest.NewRecorder()
		h.CreateBooking(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("slot_booked", func(t *testing.T) {
		st := &stubBookingAPI{createErr: service.ErrSlotBooked}
		h := New(st, discardLog())
		buf, _ := json.Marshal(map[string]string{"slotId": slotID.String()})
		req := httptest.NewRequest("POST", "/bookings/create", bytes.NewReader(buf))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-ID", uid.String())
		req.Header.Set("X-User-Role", "user")
		rec := httptest.NewRecorder()
		h.CreateBooking(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		h := New(&stubBookingAPI{}, discardLog())
		req := httptest.NewRequest("POST", "/bookings/create", bytes.NewReader([]byte(`{`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-ID", uid.String())
		req.Header.Set("X-User-Role", "user")
		rec := httptest.NewRecorder()
		h.CreateBooking(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
	})
}

func TestCancelBooking_HandlerBranches(t *testing.T) {
	uid := uuid.New()
	bid := uuid.New()

	t.Run("wrong_role", func(t *testing.T) {
		h := New(&stubBookingAPI{}, discardLog())
		req := httptest.NewRequest("POST", "/bookings/"+bid.String()+"/cancel", nil)
		req.Header.Set("X-User-ID", uid.String())
		req.Header.Set("X-User-Role", "admin")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("bookingId", bid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.CancelBooking(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		st := &stubBookingAPI{cancelErr: service.ErrBookingNotFound}
		h := New(st, discardLog())
		req := httptest.NewRequest("POST", "/x", nil)
		req.Header.Set("X-User-ID", uid.String())
		req.Header.Set("X-User-Role", "user")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("bookingId", bid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.CancelBooking(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		st := &stubBookingAPI{cancelErr: service.ErrForbidden}
		h := New(st, discardLog())
		req := httptest.NewRequest("POST", "/x", nil)
		req.Header.Set("X-User-ID", uid.String())
		req.Header.Set("X-User-Role", "user")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("bookingId", bid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.CancelBooking(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		st := &stubBookingAPI{cancelRes: &service.BookingResponse{ID: bid, Status: "cancelled"}}
		h := New(st, discardLog())
		req := httptest.NewRequest("POST", "/x", nil)
		req.Header.Set("X-User-ID", uid.String())
		req.Header.Set("X-User-Role", "user")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("bookingId", bid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.CancelBooking(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("invalid_booking_id", func(t *testing.T) {
		h := New(&stubBookingAPI{}, discardLog())
		req := httptest.NewRequest("POST", "/x", nil)
		req.Header.Set("X-User-ID", uid.String())
		req.Header.Set("X-User-Role", "user")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("bookingId", "not-uuid")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.CancelBooking(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
	})
}

func TestMyBookings_HandlerSuccess(t *testing.T) {
	uid := uuid.New()
	st := &stubBookingAPI{my: []service.BookingResponse{{ID: uuid.New(), UserID: uid, Status: "active"}}}
	h := New(st, discardLog())
	req := httptest.NewRequest("GET", "/bookings/my", nil)
	req.Header.Set("X-User-ID", uid.String())
	req.Header.Set("X-User-Role", "user")
	rec := httptest.NewRecorder()
	h.MyBookings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestListBookings_HandlerBranches(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		st := &stubBookingAPI{list: []service.BookingResponse{{ID: uuid.New(), Status: "active"}}, listTotal: 1}
		h := New(st, discardLog())
		req := httptest.NewRequest("GET", "/bookings/list?page=1&pageSize=10", nil)
		req.Header.Set("X-User-ID", uuid.New().String())
		req.Header.Set("X-User-Role", "admin")
		rec := httptest.NewRecorder()
		h.ListBookings(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("bad_pagination", func(t *testing.T) {
		h := New(&stubBookingAPI{}, discardLog())
		req := httptest.NewRequest("GET", "/bookings/list?page=0&pageSize=10", nil)
		req.Header.Set("X-User-ID", uuid.New().String())
		req.Header.Set("X-User-Role", "admin")
		rec := httptest.NewRecorder()
		h.ListBookings(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
	})
}
