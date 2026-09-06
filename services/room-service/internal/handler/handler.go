package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/room-booking/pkg/httputil"
	"github.com/room-booking/services/room-service/internal/repository"
)

// roomStore is satisfied by *repository.RoomRepository; narrowed for tests.
type roomStore interface {
	Create(ctx context.Context, name string, description *string, capacity *int, createdBy uuid.UUID) (*repository.Room, error)
	List(ctx context.Context) ([]repository.Room, error)
	GetByID(ctx context.Context, id uuid.UUID) (*repository.Room, error)
}

type Handler struct {
	repo roomStore
	log  *slog.Logger
}

func New(repo roomStore, log *slog.Logger) *Handler {
	return &Handler{repo: repo, log: log}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/_health", func(w http.ResponseWriter, _ *http.Request) {
		httputil.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	// Public (proxied from gateway)
	r.Post("/rooms/create", h.CreateRoom)
	r.Get("/rooms/list", h.ListRooms)
	// Internal
	r.Get("/internal/rooms/{roomId}", h.GetRoom)
	return r
}

func (h *Handler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		httputil.Unauthorized(w)
		return
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		httputil.BadRequest(w, "invalid user id")
		return
	}

	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Capacity    *int    `json:"capacity"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}
	if req.Name == "" {
		httputil.BadRequest(w, "name is required")
		return
	}
	if req.Capacity != nil && *req.Capacity <= 0 {
		httputil.BadRequest(w, "capacity must be positive")
		return
	}

	room, err := h.repo.Create(r.Context(), req.Name, req.Description, req.Capacity, uid)
	if err != nil {
		h.log.Error("create room failed", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]interface{}{"room": room})
}

func (h *Handler) ListRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := h.repo.List(r.Context())
	if err != nil {
		h.log.Error("list rooms failed", "error", err)
		httputil.InternalError(w)
		return
	}
	if rooms == nil {
		rooms = []repository.Room{}
	}
	httputil.JSON(w, http.StatusOK, map[string]interface{}{"rooms": rooms})
}

func (h *Handler) GetRoom(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomId")
	id, err := uuid.Parse(roomID)
	if err != nil {
		httputil.BadRequest(w, "invalid room id")
		return
	}
	room, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrRoomNotFound) {
			httputil.NotFound(w, "ROOM_NOT_FOUND", "room not found")
			return
		}
		h.log.Error("get room failed", "error", err)
		httputil.InternalError(w)
		return
	}
	httputil.JSON(w, http.StatusOK, room)
}
