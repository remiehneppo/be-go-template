package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestWorkerRunStopsOnContextCancel(t *testing.T) {
	out := &testOutbox{}
	worker := NewWorker(out, func(ctx context.Context, event Event) error {
		return nil
	}, time.Millisecond, 1)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- worker.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop after cancel")
	}
}

type testOutbox struct{}

func (o *testOutbox) Enqueue(ctx context.Context, event Event) error {
	return nil
}

func (o *testOutbox) ClaimBatch(ctx context.Context, limit int) ([]Event, error) {
	return nil, nil
}

func (o *testOutbox) MarkDone(ctx context.Context, id string) error {
	return nil
}

func (o *testOutbox) MarkFailed(ctx context.Context, id string, reason string) error {
	return nil
}
