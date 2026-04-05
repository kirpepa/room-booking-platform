package handler

import (
	"encoding/json"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/room-booking/services/conference-mock-service/internal/store"
)

type Handler struct {
	store          *store.Store
	log            *slog.Logger
	failRate       float64 // 0.0 to 1.0
	maxDelayMs     int
}

func New(s *store.Store, log *slog.Logger) *Handler {
	failRate := 0.0
	if v := os.Getenv("CONFERENCE_FAIL_RATE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			failRate = f
		}
	}
	maxDelay := 0
	if v := os.Getenv("CONFERENCE_MAX_DELAY_MS"); v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			maxDelay = d
		}
	}
	return &Handler{store: s, log: log, failRate: failRate, maxDelayMs: maxDelay}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/internal/conference/create", h.CreateConference)
	return r
}

func (h *Handler) CreateConference(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BookingID string `json:"booking_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	bookingID, err := uuid.Parse(req.BookingID)
	if err != nil {
		http.Error(w, `{"error":"invalid booking_id"}`, http.StatusBadRequest)
		return
	}

	// Simulate delay
	if h.maxDelayMs > 0 {
		delay := time.Duration(rand.Intn(h.maxDelayMs)) * time.Millisecond
		time.Sleep(delay)
	}

	// Simulate failure
	if h.failRate > 0 && rand.Float64() < h.failRate {
		h.log.Warn("simulating failure", "bookingId", bookingID)
		http.Error(w, `{"error":"service unavailable"}`, http.StatusInternalServerError)
		return
	}

	// Idempotent: same booking_id always returns same link
	link := h.store.GetOrCreate(bookingID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"link": link})
}
