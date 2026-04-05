package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/room-booking/services/booking-service/internal/service"
)

type ConferenceWorker struct {
	svc      *service.BookingService
	log      *slog.Logger
	interval time.Duration
}

func NewConferenceWorker(svc *service.BookingService, log *slog.Logger) *ConferenceWorker {
	return &ConferenceWorker{
		svc:      svc,
		log:      log,
		interval: 5 * time.Second,
	}
}

func (w *ConferenceWorker) Start(ctx context.Context) {
	w.log.Info("conference worker started")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("conference worker stopped")
			return
		case <-ticker.C:
			if err := w.svc.ProcessConferenceJobs(ctx); err != nil {
				w.log.Error("process conference jobs failed", "error", err)
			}
		}
	}
}
