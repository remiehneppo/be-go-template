package outbox

import (
	"context"
	"time"
)

type Handler func(ctx context.Context, event Event) error

type Worker struct {
	outbox   Outbox
	handler  Handler
	interval time.Duration
	batch    int
}

func NewWorker(outbox Outbox, handler Handler, interval time.Duration, batch int) *Worker {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if batch <= 0 {
		batch = 10
	}
	return &Worker{outbox: outbox, handler: handler, interval: interval, batch: batch}
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		if err := w.DrainOnce(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *Worker) DrainOnce(ctx context.Context) error {
	events, err := w.outbox.ClaimBatch(ctx, w.batch)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := w.handler(ctx, event); err != nil {
			if markErr := w.outbox.MarkFailed(ctx, event.ID, err.Error()); markErr != nil {
				return markErr
			}
			continue
		}
		if err := w.outbox.MarkDone(ctx, event.ID); err != nil {
			return err
		}
	}
	return nil
}
