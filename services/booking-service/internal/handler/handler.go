package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/room-booking/pkg/httputil"
	"github.com/room-booking/services/booking-service/internal/service"
)

// bookingAPI is implemented by *service.BookingService; narrowed for handler tests.
type bookingAPI interface {
	CreateBooking(ctx context.Context, userID uuid.UUID, role string, req service.CreateBookingRequest) (*service.BookingResponse, error)
	CancelBooking(ctx context.Context, bookingID, userID uuid.UUID) (*service.BookingResponse, error)
	GetMyBookings(ctx context.Context, userID uuid.UUID) ([]service.BookingResponse, error)
	ListBookings(ctx context.Context, page, pageSize int) ([]service.BookingResponse, int, error)
}

type Handler struct {
	svc bookingAPI
	log *slog.Logger
}

func New(svc bookingAPI, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/bookings/create", h.CreateBooking)
	r.Get("/bookings/list", h.ListBookings)
	r.Get("/bookings/my", h.MyBookings)
	r.Post("/bookings/{bookingId}/cancel", h.CancelBooking)
	return r
}

func (h *Handler) CreateBooking(w http.ResponseWriter, r *http.Request) {
	userID, role := extractAuth(r)
	if userID == uuid.Nil {
		httputil.Unauthorized(w)
		return
	}

	if role == "admin" {
		httputil.Forbidden(w, "admin cannot create bookings")
		return
	}

	var req service.CreateBookingRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}

	booking, err := h.svc.CreateBooking(r.Context(), userID, role, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAdminCannotBook):
			httputil.Forbidden(w, "admin cannot create bookings")
		case errors.Is(err, service.ErrSlotNotFound):
			httputil.NotFound(w, "SLOT_NOT_FOUND", "slot not found")
		case errors.Is(err, service.ErrSlotInPast):
			httputil.BadRequest(w, "cannot book slot in the past")
		case errors.Is(err, service.ErrSlotBooked):
			httputil.Conflict(w, "SLOT_ALREADY_BOOKED", "slot is already booked")
		default:
			h.log.Error("create booking failed", "error", err)
			httputil.InternalError(w)
		}
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]interface{}{"booking": booking})
}

func (h *Handler) CancelBooking(w http.ResponseWriter, r *http.Request) {
	userID, role := extractAuth(r)
	if userID == uuid.Nil {
		httputil.Unauthorized(w)
		return
	}
	if role != "user" {
		httputil.Forbidden(w, "only user can cancel bookings")
		return
	}

	bookingIDStr := chi.URLParam(r, "bookingId")
	bookingID, err := uuid.Parse(bookingIDStr)
	if err != nil {
		httputil.BadRequest(w, "invalid booking id")
		return
	}

	booking, err := h.svc.CancelBooking(r.Context(), bookingID, userID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrBookingNotFound):
			httputil.NotFound(w, "BOOKING_NOT_FOUND", "booking not found")
		case errors.Is(err, service.ErrForbidden):
			httputil.Forbidden(w, "cannot cancel another user's booking")
		default:
			h.log.Error("cancel booking failed", "error", err)
			httputil.InternalError(w)
		}
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]interface{}{"booking": booking})
}

func (h *Handler) MyBookings(w http.ResponseWriter, r *http.Request) {
	userID, role := extractAuth(r)
	if userID == uuid.Nil {
		httputil.Unauthorized(w)
		return
	}
	if role != "user" {
		httputil.Forbidden(w, "only user can view their bookings")
		return
	}

	bookings, err := h.svc.GetMyBookings(r.Context(), userID)
	if err != nil {
		h.log.Error("get my bookings failed", "error", err)
		httputil.InternalError(w)
		return
	}
	if bookings == nil {
		bookings = []service.BookingResponse{}
	}
	httputil.JSON(w, http.StatusOK, map[string]interface{}{"bookings": bookings})
}

func (h *Handler) ListBookings(w http.ResponseWriter, r *http.Request) {
	_, role := extractAuth(r)
	if role != "admin" {
		httputil.Forbidden(w, "only admin can list all bookings")
		return
	}

	page := queryInt(r, "page", 1)
	pageSize := queryInt(r, "pageSize", 20)

	if page < 1 || pageSize < 1 || pageSize > 100 {
		httputil.BadRequest(w, "invalid pagination parameters")
		return
	}

	bookings, total, err := h.svc.ListBookings(r.Context(), page, pageSize)
	if err != nil {
		h.log.Error("list bookings failed", "error", err)
		httputil.InternalError(w)
		return
	}
	if bookings == nil {
		bookings = []service.BookingResponse{}
	}

	httputil.JSON(w, http.StatusOK, map[string]interface{}{
		"bookings": bookings,
		"pagination": map[string]int{
			"page":     page,
			"pageSize": pageSize,
			"total":    total,
		},
	})
}

func extractAuth(r *http.Request) (uuid.UUID, string) {
	uidStr := r.Header.Get("X-User-ID")
	role := r.Header.Get("X-User-Role")
	uid, err := uuid.Parse(uidStr)
	if err != nil {
		return uuid.Nil, ""
	}
	return uid, role
}

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
