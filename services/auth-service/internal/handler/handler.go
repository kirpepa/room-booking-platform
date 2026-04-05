package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/room-booking/pkg/httputil"
	"github.com/room-booking/services/auth-service/internal/service"
)

type Handler struct {
	svc *service.AuthService
	log *slog.Logger
}

func New(svc *service.AuthService, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/register", h.Register)
	r.Post("/login", h.Login)
	r.Post("/dummyLogin", h.DummyLogin)
	r.Post("/seed", h.Seed)
	return r
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
