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
	"github.com/room-booking/services/room-service/internal/repository"
)

type stubRoomStore struct {
	room *repository.Room
	cErr error
	list []repository.Room
	lErr error
	gErr error
}

func (s *stubRoomStore) Create(ctx context.Context, name string, description *string, capacity *int, createdBy uuid.UUID) (*repository.Room, error) {
	if s.cErr != nil {
		return nil, s.cErr
	}
	if s.room != nil {
		return s.room, nil
	}
	return &repository.Room{ID: uuid.New(), Name: name, CreatedBy: createdBy}, nil
}

func (s *stubRoomStore) List(ctx context.Context) ([]repository.Room, error) {
	if s.lErr != nil {
		return nil, s.lErr
	}
	return s.list, nil
}

func (s *stubRoomStore) GetByID(ctx context.Context, id uuid.UUID) (*repository.Room, error) {
	if s.gErr != nil {
		return nil, s.gErr
	}
	if s.room != nil && s.room.ID == id {
		return s.room, nil
	}
	return nil, repository.ErrRoomNotFound
}

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCreateRoom_Handler(t *testing.T) {
	t.Run("no_auth", func(t *testing.T) {
		h := New(&stubRoomStore{}, discardLog())
		rec := httptest.NewRecorder()
		h.CreateRoom(rec, httptest.NewRequest("POST", "/rooms/create", bytes.NewReader([]byte(`{"name":"A"}`))))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("invalid_user_id", func(t *testing.T) {
		h := New(&stubRoomStore{}, discardLog())
		req := httptest.NewRequest("POST", "/rooms/create", bytes.NewReader([]byte(`{"name":"A"}`)))
		req.Header.Set("X-User-ID", "bad")
		rec := httptest.NewRecorder()
		h.CreateRoom(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("empty_name", func(t *testing.T) {
		h := New(&stubRoomStore{}, discardLog())
		body, _ := json.Marshal(map[string]string{"name": ""})
		req := httptest.NewRequest("POST", "/rooms/create", bytes.NewReader(body))
		req.Header.Set("X-User-ID", uuid.New().String())
		rec := httptest.NewRecorder()
		h.CreateRoom(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("bad_capacity", func(t *testing.T) {
		h := New(&stubRoomStore{}, discardLog())
		cap := 0
		body, _ := json.Marshal(map[string]interface{}{"name": "R", "capacity": &cap})
		req := httptest.NewRequest("POST", "/rooms/create", bytes.NewReader(body))
		req.Header.Set("X-User-ID", uuid.New().String())
		rec := httptest.NewRecorder()
		h.CreateRoom(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("created", func(t *testing.T) {
		uid := uuid.New()
		r := &repository.Room{ID: uuid.New(), Name: "Board", CreatedBy: uid}
		h := New(&stubRoomStore{room: r}, discardLog())
		body, _ := json.Marshal(map[string]string{"name": "Board"})
		req := httptest.NewRequest("POST", "/rooms/create", bytes.NewReader(body))
		req.Header.Set("X-User-ID", uid.String())
		rec := httptest.NewRecorder()
		h.CreateRoom(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		h := New(&stubRoomStore{cErr: io.ErrClosedPipe}, discardLog())
		body, _ := json.Marshal(map[string]string{"name": "R"})
		req := httptest.NewRequest("POST", "/rooms/create", bytes.NewReader(body))
		req.Header.Set("X-User-ID", uuid.New().String())
		rec := httptest.NewRecorder()
		h.CreateRoom(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("code %d", rec.Code)
		}
	})
}

func TestListRooms_Handler(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		h := New(&stubRoomStore{list: []repository.Room{{ID: uuid.New(), Name: "A"}}}, discardLog())
		rec := httptest.NewRecorder()
		h.ListRooms(rec, httptest.NewRequest("GET", "/rooms/list", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("code %d", rec.Code)
		}
	})
	t.Run("repo_error", func(t *testing.T) {
		h := New(&stubRoomStore{lErr: io.ErrClosedPipe}, discardLog())
		rec := httptest.NewRecorder()
		h.ListRooms(rec, httptest.NewRequest("GET", "/rooms/list", nil))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("code %d", rec.Code)
		}
	})
}

func TestGetRoom_Handler(t *testing.T) {
	rid := uuid.New()
	room := &repository.Room{ID: rid, Name: "X"}

	t.Run("not_found", func(t *testing.T) {
		h := New(&stubRoomStore{}, discardLog())
		req := httptest.NewRequest("GET", "/internal/rooms/"+rid.String(), nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("roomId", rid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.GetRoom(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("ok", func(t *testing.T) {
		h := New(&stubRoomStore{room: room}, discardLog())
		req := httptest.NewRequest("GET", "/internal/rooms/"+rid.String(), nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("roomId", rid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.GetRoom(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code %d", rec.Code)
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		h := New(&stubRoomStore{gErr: io.ErrClosedPipe}, discardLog())
		req := httptest.NewRequest("GET", "/internal/rooms/"+rid.String(), nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("roomId", rid.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.GetRoom(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("code %d", rec.Code)
		}
	})
}
