package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/room-booking/pkg/httputil"
	"github.com/room-booking/services/auth-service/internal/service"
)

type authAPI interface {
	Register(ctx context.Context, email, password, role string) (*service.UserResponse, error)
	Login(ctx context.Context, email, password string) (string, error)
	DummyLogin(ctx context.Context, role string) (string, error)
	Seed(ctx context.Context) error
}

type Handler struct {
	svc          authAPI
	log          *slog.Logger
	testTaskMode bool
}

func New(svc authAPI, log *slog.Logger, testTaskMode bool) *Handler {
	return &Handler{svc: svc, log: log, testTaskMode: testTaskMode}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/_health", h.Health)
	r.Post("/register", h.Register)
	r.Post("/login", h.Login)
	if h.testTaskMode {
		r.Post("/dummyLogin", h.DummyLogin)
		r.Post("/seed", h.Seed)
	}
	return r
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" || req.Role == "" {
		httputil.BadRequest(w, "email, password and role are required")
		return
	}
	if req.Role == "admin" && !h.testTaskMode {
		httputil.Forbidden(w, "admin accounts cannot be self-registered")
		return
	}

	user, err := h.svc.Register(r.Context(), req.Email, req.Password, req.Role)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRole) {
			httputil.BadRequest(w, "role must be admin or user")
			return
		}
		if errors.Is(err, service.ErrEmailExists) {
			httputil.BadRequest(w, "email already exists")
			return
		}
		if errors.Is(err, service.ErrInvalidEmail) || errors.Is(err, service.ErrInvalidPassword) {
			httputil.BadRequest(w, err.Error())
			return
		}
		h.log.Error("register failed", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]interface{}{"user": user})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}

	token, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			httputil.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
			return
		}
		h.log.Error("login failed", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"token": token})
}

func (h *Handler) DummyLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role string `json:"role"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}

	token, err := h.svc.DummyLogin(r.Context(), req.Role)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRole) {
			httputil.BadRequest(w, "role must be admin or user")
			return
		}
		h.log.Error("dummyLogin failed", "error", err)
		httputil.InternalError(w)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"token": token})
}

func (h *Handler) Seed(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Seed(r.Context()); err != nil {
		h.log.Error("seed failed", "error", err)
		httputil.InternalError(w)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
