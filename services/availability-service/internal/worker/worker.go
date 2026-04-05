package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/room-booking/services/availability-service/internal/service"
)

type SlotGenerationWorker struct {
	svc      *service.AvailabilityService
	log      *slog.Logger
	interval time.Duration
}

func NewSlotGenerationWorker(svc *service.AvailabilityService, log *slog.Logger) *SlotGenerationWorker {
	return &SlotGenerationWorker{
		svc:      svc,
		log:      log,
		interval: 1 * time.Hour,
	}
}

func (w *SlotGenerationWorker) Start(ctx context.Context) {
	w.log.Info("slot generation worker started", "interval", w.interval)

	// Run immediately on start
	if err := w.svc.ExtendSlots(ctx); err != nil {
		w.log.Error("initial slot extension failed", "error", err)
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("slot generation worker stopped")
			return
		case <-ticker.C:
			if err := w.svc.ExtendSlots(ctx); err != nil {
				w.log.Error("slot extension failed", "error", err)
			}
		}
	}
}
