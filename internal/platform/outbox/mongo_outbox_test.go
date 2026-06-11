package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/remihneppo/be-go-template/internal/platform/database"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMongoOutboxEnqueueDefaults(t *testing.T) {
	db := &fakeDatabase{}
	out := NewMongoOutbox(db)
	out.now = func() time.Time { return time.Unix(10, 0).UTC() }

	err := out.Enqueue(context.Background(), Event{ID: "e1", IdempotencyKey: "k1", Type: "audit"})
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	doc := db.inserted.(eventDocument)
	if doc.Status != StatusPending || doc.MaxRetries != 10 || !doc.ProcessAfter.Equal(time.Unix(10, 0).UTC()) {
		t.Fatalf("document = %+v", doc)
	}
}

func TestMongoOutboxClaimBatchMarksProcessing(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	db := &fakeDatabase{findManyValue: []eventDocument{{ID: "e1", Status: StatusPending, MaxRetries: 3, ProcessAfter: now}}}
	out := NewMongoOutbox(db)
	out.now = func() time.Time { return now }

	events, err := out.ClaimBatch(context.Background(), 5)
	if err != nil {
		t.Fatalf("ClaimBatch() error = %v", err)
	}
	if len(events) != 1 || events[0].Status != StatusProcessing {
		t.Fatalf("events = %+v", events)
	}
	if db.updateOneCalls != 1 {
		t.Fatalf("updateOneCalls = %d", db.updateOneCalls)
	}
}

func TestMongoOutboxMarkFailedSchedulesRetry(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	db := &fakeDatabase{}
	out := NewMongoOutbox(db)
	out.now = func() time.Time { return now }

	if err := out.MarkFailed(context.Background(), "e1", "boom"); err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}
	update, ok := db.lastUpdate.(bson.M)
	if !ok {
		t.Fatalf("update type = %T", db.lastUpdate)
	}
	if update["$inc"] == nil || update["$set"] == nil {
		t.Fatalf("update = %#v", db.lastUpdate)
	}
}

func TestWorkerDrainOnceMarksDoneAndFailed(t *testing.T) {
	out := &fakeOutbox{events: []Event{{ID: "ok"}, {ID: "bad"}}}
	worker := NewWorker(out, func(ctx context.Context, event Event) error {
		if event.ID == "bad" {
			return errors.New("boom")
		}
		return nil
	}, time.Second, 10)

	if err := worker.DrainOnce(context.Background()); err != nil {
		t.Fatalf("DrainOnce() error = %v", err)
	}
	if out.doneID != "ok" || out.failedID != "bad" {
		t.Fatalf("done=%q failed=%q", out.doneID, out.failedID)
	}
}

type fakeDatabase struct {
	inserted       any
	findManyValue  any
	lastUpdate     any
	updateOneCalls int
}

func (d *fakeDatabase) FindOne(ctx context.Context, collection string, filter any, dest any, opts database.ReadOptions) error {
	return database.ErrNotFound
}

func (d *fakeDatabase) FindMany(ctx context.Context, collection string, filter any, dest any, opts database.ReadOptions) error {
	return copyValue(dest, d.findManyValue)
}

func (d *fakeDatabase) InsertOne(ctx context.Context, collection string, document any, opts database.WriteOptions) error {
	d.inserted = document
	return nil
}

func (d *fakeDatabase) UpdateOne(ctx context.Context, collection string, filter any, update any, opts database.WriteOptions) error {
	d.updateOneCalls++
	d.lastUpdate = update
	return nil
}

func (d *fakeDatabase) UpdateMany(ctx context.Context, collection string, filter any, update any, opts database.WriteOptions) error {
	return nil
}

func (d *fakeDatabase) DeleteOne(ctx context.Context, collection string, filter any, opts database.WriteOptions) error {
	return nil
}

func (d *fakeDatabase) Count(ctx context.Context, collection string, filter any) (int64, error) {
	return 0, nil
}

func (d *fakeDatabase) Ping(ctx context.Context) error {
	return nil
}

func (d *fakeDatabase) Close(ctx context.Context) error {
	return nil
}

func copyValue(dest any, src any) error {
	payload, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, dest)
}

type fakeOutbox struct {
	events   []Event
	doneID   string
	failedID string
}

func (o *fakeOutbox) Enqueue(ctx context.Context, event Event) error {
	o.events = append(o.events, event)
	return nil
}

func (o *fakeOutbox) ClaimBatch(ctx context.Context, limit int) ([]Event, error) {
	return o.events, nil
}

func (o *fakeOutbox) MarkDone(ctx context.Context, id string) error {
	o.doneID = id
	return nil
}

func (o *fakeOutbox) MarkFailed(ctx context.Context, id string, reason string) error {
	o.failedID = id
	return nil
}
