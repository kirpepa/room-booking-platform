package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pkgjwt "github.com/room-booking/pkg/jwt"
	"github.com/room-booking/services/auth-service/internal/config"
	"github.com/room-booking/services/auth-service/internal/handler"
	"github.com/room-booking/services/auth-service/internal/repository"
	"github.com/room-booking/services/auth-service/internal/service"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Error("failed to ping database", "error", err)
		os.Exit(1)
	}

	privateKey, err := pkgjwt.LoadPrivateKey(cfg.PrivateKeyPath)
	if err != nil {
		log.Error("failed to load private key", "error", err)
		os.Exit(1)
	}

	repo := repository.NewUserRepository(pool)
	svc := service.NewAuthService(repo, privateKey)
	h := handler.New(svc, log)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.Port),
		Handler: h.Routes(),
	}

	go func() {
		log.Info("auth-service starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
	log.Info("auth-service stopped")
}
