package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	pkgjwt "github.com/room-booking/pkg/jwt"
	"github.com/room-booking/services/api-gateway/internal/config"
	"github.com/room-booking/services/api-gateway/internal/middleware"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	publicKey, err := pkgjwt.LoadPublicKey(cfg.PublicKeyPath)
	if err != nil {
		log.Error("failed to load public key", "error", err)
		os.Exit(1)
	}

	auth := middleware.NewAuthMiddleware(publicKey)

	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))

	// /_info - always 200
	r.Get("/_info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"room-booking"}`))
	})

	// Auth endpoints - no auth required
	r.Post("/register", proxyTo(cfg.AuthServiceURL, "/register"))
	r.Post("/login", proxyTo(cfg.AuthServiceURL, "/login"))
	r.Post("/dummyLogin", proxyTo(cfg.AuthServiceURL, "/dummyLogin"))

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth)

		// Rooms - admin and user
		r.Get("/rooms/list", proxyTo(cfg.RoomServiceURL, "/rooms/list"))

		// Rooms - admin only
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireRole("admin"))
			r.Post("/rooms/create", proxyTo(cfg.RoomServiceURL, "/rooms/create"))
		})

		// Schedule - admin only
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireRole("admin"))
			r.Post("/rooms/{roomId}/schedule/create", proxyToWithPathRewrite(cfg.AvailabilityServiceURL))
		})

		// Slots - admin and user
		r.Get("/rooms/{roomId}/slots/list", proxyToWithPathRewrite(cfg.AvailabilityServiceURL))

		// Bookings
		r.Post("/bookings/create", proxyTo(cfg.BookingServiceURL, "/bookings/create"))
		r.Get("/bookings/my", proxyTo(cfg.BookingServiceURL, "/bookings/my"))

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireRole("admin"))
			r.Get("/bookings/list", proxyTo(cfg.BookingServiceURL, "/bookings/list"))
		})

		r.Post("/bookings/{bookingId}/cancel", proxyToWithPathRewrite(cfg.BookingServiceURL))
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.Port),
		Handler: r,
	}

	go func() {
		log.Info("api-gateway starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("api-gateway stopped")
}

// proxyTo creates a handler that proxies the entire request to upstream with the given path
func proxyTo(upstream, path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target, err := url.Parse(upstream)
		if err != nil {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}

		proxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = target.Scheme
				req.URL.Host = target.Host
				req.URL.Path = path
				req.URL.RawQuery = r.URL.RawQuery
				req.Host = target.Host
				// Preserve X-User-ID and X-User-Role headers
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				io.WriteString(w, `{"error":{"code":"INTERNAL_ERROR","message":"service unavailable"}}`)
			},
		}
		proxy.ServeHTTP(w, r)
	}
}

// proxyToWithPathRewrite proxies while keeping the original path (for routes with path params)
func proxyToWithPathRewrite(upstream string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target, err := url.Parse(upstream)
		if err != nil {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}

		// Resolve chi URL params into the path
		rctx := chi.RouteContext(r.Context())
		originalPath := r.URL.Path
		if rctx != nil {
			// The path is already correct from chi routing
			_ = rctx
		}

		proxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = target.Scheme
				req.URL.Host = target.Host
				req.URL.Path = originalPath
				req.URL.RawQuery = r.URL.RawQuery
				req.Host = target.Host
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				io.WriteString(w, `{"error":{"code":"INTERNAL_ERROR","message":"service unavailable"}}`)
			},
		}
		proxy.ServeHTTP(w, r)
	}
}

// stripPrefix helper (unused but available)
func stripPrefix(prefix, path string) string {
	return strings.TrimPrefix(path, prefix)
}
