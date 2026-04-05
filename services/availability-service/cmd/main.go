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
	"github.com/room-booking/services/availability-service/internal/config"
	"github.com/room-booking/services/availability-service/internal/handler"
	"github.com/room-booking/services/availability-service/internal/repository"
	"github.com/room-booking/services/availability-service/internal/service"
	"github.com/room-booking/services/availability-service/internal/worker"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	repo := repository.New(pool)
	svc := service.New(repo, log, cfg.RoomServiceURL)
	h := handler.New(svc, log)

	// Start slot generation worker
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	w := worker.NewSlotGenerationWorker(svc, log)
	go w.Start(workerCtx)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.Port),
		Handler: h.Routes(),
	}

	go func() {
		log.Info("availability-service starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	workerCancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
	log.Info("availability-service stopped")
}
