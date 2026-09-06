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
	"github.com/room-booking/services/availability-service/internal/repository"
	"github.com/room-booking/services/availability-service/internal/service"
)

type stubAvailSvc struct {
	sched    *repository.Schedule
	schedErr error
	slots    []repository.Slot
	slotsErr error
	meta     *repository.SlotMeta
	bookErr  error
	relErr   error
	slot     *repository.Slot
	slotErr  error
}

func (s *stubAvailSvc) CreateSchedule(ctx context.Context, pathRoomID uuid.UUID, req service.CreateScheduleRequest) (*repository.Schedule, error) {
	return s.sched, s.schedErr
}

func (s *stubAvailSvc) GetFreeSlots(ctx context.Context, roomID uuid.UUID, dateStr string) ([]repository.Slot, error) {
	return s.slots, s.slotsErr
}

func (s *stubAvailSvc) BookSlot(ctx context.Context, slotID, bookingID uuid.UUID) (*repository.SlotMeta, error) {
	return s.meta, s.bookErr
}

func (s *stubAvailSvc) ReleaseSlot(ctx context.Context, slotID, bookingID uuid.UUID) error {
	return s.relErr
}

func (s *stubAvailSvc) GetSlot(ctx context.Context, slotID uuid.UUID) (*repository.Slot, error) {
	return s.slot, s.slotErr
}

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCreateSchedule_Handler(t *testing.T) {
	rid := uuid.New()
	t.Run("invalid_room_path", func(t *testing.T) {
		h := New(&stubAvailSvc{}, discardLog())
		req := httptest.NewRequest("POST", "/rooms/bad/schedule/create", bytes.NewReader([]byte(`{}`)))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("roomId", "not-uuid")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.CreateSchedule(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("invalid_request_from_service", func(t *testing.T) {
		h := New(&stubAvailSvc{schedErr: service.ErrInvalidRequest}, discardLog())
		body, _ := json.Marshal(map[string]interface{}{
			"roomId": rid.String(), "daysOfWeek": []int{1}, "startTime": "09:00", "endTime": "10:00",
		})
		req := httptest.NewRequest("POST", "/x", bytes.NewReader(body))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("roomId", rid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.CreateSchedule(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		h := New(&stubAvailSvc{}, discardLog())
		req := httptest.NewRequest("POST", "/x", bytes.NewReader([]byte(`{`)))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("roomId", rid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.CreateSchedule(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("conflict", func(t *testing.T) {
		h := New(&stubAvailSvc{schedErr: service.ErrScheduleExists}, discardLog())
		body, _ := json.Marshal(map[string]interface{}{
			"roomId": rid.String(), "daysOfWeek": []int{1}, "startTime": "09:00", "endTime": "10:00",
		})
		req := httptest.NewRequest("POST", "/x", bytes.NewReader(body))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("roomId", rid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.CreateSchedule(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("room_not_found", func(t *testing.T) {
		h := New(&stubAvailSvc{schedErr: service.ErrRoomNotFound}, discardLog())
		body, _ := json.Marshal(map[string]interface{}{
			"roomId": rid.String(), "daysOfWeek": []int{1}, "startTime": "09:00", "endTime": "10:00",
		})
		req := httptest.NewRequest("POST", "/x", bytes.NewReader(body))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("roomId", rid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.CreateSchedule(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("created", func(t *testing.T) {
		sched := &repository.Schedule{ID: uuid.New(), RoomID: rid, StartTime: "09:00", EndTime: "10:00"}
		h := New(&stubAvailSvc{sched: sched}, discardLog())
		body, _ := json.Marshal(map[string]interface{}{
			"roomId": rid.String(), "daysOfWeek": []int{1}, "startTime": "09:00", "endTime": "10:00",
		})
		req := httptest.NewRequest("POST", "/x", bytes.NewReader(body))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("roomId", rid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.CreateSchedule(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("code %d", rec.Code)
		}
	})
}

func TestListSlots_Handler(t *testing.T) {
	rid := uuid.New()
	t.Run("missing_date", func(t *testing.T) {
		h := New(&stubAvailSvc{}, discardLog())
		req := httptest.NewRequest("GET", "/rooms/"+rid.String()+"/slots/list", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("roomId", rid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.ListSlots(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("invalid_request", func(t *testing.T) {
		h := New(&stubAvailSvc{slotsErr: service.ErrInvalidRequest}, discardLog())
		req := httptest.NewRequest("GET", "/rooms/"+rid.String()+"/slots/list?date=2030-01-01", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("roomId", rid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.ListSlots(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("ok", func(t *testing.T) {
		h := New(&stubAvailSvc{slots: []repository.Slot{{ID: uuid.New()}}}, discardLog())
		req := httptest.NewRequest("GET", "/rooms/"+rid.String()+"/slots/list?date=2030-01-01", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("roomId", rid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.ListSlots(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("room_not_found", func(t *testing.T) {
		h := New(&stubAvailSvc{slotsErr: service.ErrRoomNotFound}, discardLog())
		req := httptest.NewRequest("GET", "/rooms/"+rid.String()+"/slots/list?date=2030-01-01", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("roomId", rid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.ListSlots(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("code %d", rec.Code)
		}
	})
}

func TestReleaseSlot_HandlerOK(t *testing.T) {
	sid, bid := uuid.New(), uuid.New()
	body, _ := json.Marshal(map[string]string{"slot_id": sid.String(), "booking_id": bid.String()})
	h := New(&stubAvailSvc{}, discardLog())
	req := httptest.NewRequest("POST", "/internal/slots/release", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ReleaseSlot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestBookSlot_HandlerErrors(t *testing.T) {
	sid, bid := uuid.New(), uuid.New()
	body, _ := json.Marshal(map[string]string{"slot_id": sid.String(), "booking_id": bid.String()})

	t.Run("not_found", func(t *testing.T) {
		h := New(&stubAvailSvc{bookErr: repository.ErrSlotNotFound}, discardLog())
		req := httptest.NewRequest("POST", "/internal/slots/book", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		h.BookSlot(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("in_past", func(t *testing.T) {
		h := New(&stubAvailSvc{bookErr: repository.ErrSlotInPast}, discardLog())
		req := httptest.NewRequest("POST", "/internal/slots/book", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		h.BookSlot(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("booked", func(t *testing.T) {
		h := New(&stubAvailSvc{bookErr: repository.ErrSlotBooked}, discardLog())
		req := httptest.NewRequest("POST", "/internal/slots/book", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		h.BookSlot(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("ok", func(t *testing.T) {
		meta := &repository.SlotMeta{SlotID: sid, RoomID: uuid.New()}
		h := New(&stubAvailSvc{meta: meta}, discardLog())
		req := httptest.NewRequest("POST", "/internal/slots/book", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		h.BookSlot(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code %d", rec.Code)
		}
	})
}

func TestGetSlot_Handler(t *testing.T) {
	sid := uuid.New()
	slot := &repository.Slot{ID: sid}

	t.Run("invalid_id", func(t *testing.T) {
		h := New(&stubAvailSvc{}, discardLog())
		req := httptest.NewRequest("GET", "/x", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("slotId", "bad")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.GetSlot(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		h := New(&stubAvailSvc{slotErr: repository.ErrSlotNotFound}, discardLog())
		req := httptest.NewRequest("GET", "/internal/slots/"+sid.String(), nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("slotId", sid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.GetSlot(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("ok", func(t *testing.T) {
		h := New(&stubAvailSvc{slot: slot}, discardLog())
		req := httptest.NewRequest("GET", "/internal/slots/"+sid.String(), nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("slotId", sid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.GetSlot(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code %d", rec.Code)
		}
	})
}
