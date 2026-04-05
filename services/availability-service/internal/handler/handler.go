package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/room-booking/pkg/httputil"
	"github.com/room-booking/services/availability-service/internal/repository"
	"github.com/room-booking/services/availability-service/internal/service"
)

// availabilityAPI is implemented by *service.AvailabilityService; narrowed for tests.
type availabilityAPI interface {
	CreateSchedule(ctx context.Context, pathRoomID uuid.UUID, req service.CreateScheduleRequest) (*repository.Schedule, error)
	GetFreeSlots(ctx context.Context, roomID uuid.UUID, dateStr string) ([]repository.Slot, error)
	BookSlot(ctx context.Context, slotID, bookingID uuid.UUID) (*repository.SlotMeta, error)
	ReleaseSlot(ctx context.Context, slotID, bookingID uuid.UUID) error
	GetSlot(ctx context.Context, slotID uuid.UUID) (*repository.Slot, error)
}

type Handler struct {
	svc availabilityAPI
	log *slog.Logger
}

func New(svc availabilityAPI, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	// Public (proxied from gateway)
	r.Post("/rooms/{roomId}/schedule/create", h.CreateSchedule)
	r.Get("/rooms/{roomId}/slots/list", h.ListSlots)

	// Internal
	r.Post("/internal/slots/book", h.BookSlot)
	r.Post("/internal/slots/release", h.ReleaseSlot)
	r.Get("/internal/slots/{slotId}", h.GetSlot)

	return r
}

func (h *Handler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	roomIDStr := chi.URLParam(r, "roomId")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		httputil.BadRequest(w, "invalid room id")
		return
	}

	var req service.CreateScheduleRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}

	schedule, err := h.svc.CreateSchedule(r.Context(), roomID, req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRequest) {
			httputil.BadRequest(w, err.Error())
			return
		}
		if errors.Is(err, service.ErrScheduleExists) {
			httputil.Conflict(w, "SCHEDULE_EXISTS", "schedule for this room already exists and cannot be changed")
			return
		}
		if errors.Is(err, service.ErrRoomNotFound) {
			httputil.NotFound(w, "ROOM_NOT_FOUND", "room not found")
			return
		}
		h.log.Error("create schedule failed", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]interface{}{"schedule": schedule})
}

func (h *Handler) ListSlots(w http.ResponseWriter, r *http.Request) {
	roomIDStr := chi.URLParam(r, "roomId")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		httputil.BadRequest(w, "invalid room id")
		return
	}

	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		httputil.BadRequest(w, "date parameter is required")
		return
	}

	slots, err := h.svc.GetFreeSlots(r.Context(), roomID, dateStr)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRequest) {
			httputil.BadRequest(w, err.Error())
			return
		}
		if errors.Is(err, service.ErrRoomNotFound) {
			httputil.NotFound(w, "ROOM_NOT_FOUND", "room not found")
			return
		}
		h.log.Error("list slots failed", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]interface{}{"slots": slots})
}

func (h *Handler) BookSlot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SlotID    uuid.UUID `json:"slot_id"`
		BookingID uuid.UUID `json:"booking_id"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}

	meta, err := h.svc.BookSlot(r.Context(), req.SlotID, req.BookingID)
	if err != nil {
		if errors.Is(err, repository.ErrSlotNotFound) {
			httputil.NotFound(w, "SLOT_NOT_FOUND", "slot not found")
			return
		}
		if errors.Is(err, repository.ErrSlotInPast) {
			httputil.BadRequest(w, "cannot book slot in the past")
			return
		}
		if errors.Is(err, repository.ErrSlotBooked) {
			httputil.Conflict(w, "SLOT_ALREADY_BOOKED", "slot is already booked")
			return
		}
		h.log.Error("book slot failed", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.JSON(w, http.StatusOK, meta)
}

func (h *Handler) ReleaseSlot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SlotID    uuid.UUID `json:"slot_id"`
		BookingID uuid.UUID `json:"booking_id"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}

	if err := h.svc.ReleaseSlot(r.Context(), req.SlotID, req.BookingID); err != nil {
		h.log.Error("release slot failed", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) GetSlot(w http.ResponseWriter, r *http.Request) {
	slotIDStr := chi.URLParam(r, "slotId")
	slotID, err := uuid.Parse(slotIDStr)
	if err != nil {
		httputil.BadRequest(w, "invalid slot id")
		return
	}

	slot, err := h.svc.GetSlot(r.Context(), slotID)
	if err != nil {
		if errors.Is(err, repository.ErrSlotNotFound) {
			httputil.NotFound(w, "SLOT_NOT_FOUND", "slot not found")
			return
		}
		h.log.Error("get slot failed", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.JSON(w, http.StatusOK, slot)
}
