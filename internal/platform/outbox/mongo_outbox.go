package outbox

import (
	"context"
	"time"

	"github.com/remihneppo/be-go-template/internal/platform/database"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const collectionName = "outbox_events"

type MongoOutbox struct {
	db  database.Database
	now func() time.Time
}

func NewMongoOutbox(db database.Database) *MongoOutbox {
	return &MongoOutbox{
		db:  db,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (o *MongoOutbox) Enqueue(ctx context.Context, event Event) error {
	now := o.now()
	if event.Status == "" {
		event.Status = StatusPending
	}
	if event.MaxRetries <= 0 {
		event.MaxRetries = 10
	}
	if event.ProcessAfter.IsZero() {
		event.ProcessAfter = now
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	event.UpdatedAt = now
	return o.db.InsertOne(ctx, collectionName, documentFromEvent(event), database.WriteOptions{})
}

func (o *MongoOutbox) ClaimBatch(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 10
	}
	now := o.now()
	var docs []eventDocument
	if err := o.db.FindMany(ctx, collectionName, bson.M{
		"status":        bson.M{"$in": []Status{StatusPending, StatusFailed}},
		"process_after": bson.M{"$lte": now},
		"$expr":         bson.M{"$lt": []string{"$retry_count", "$max_retries"}},
	}, &docs, database.ReadOptions{
		Limit: int64(limit),
		Sort:  bson.M{"process_after": 1, "created_at": 1},
	}); err != nil {
		return nil, err
	}

	events := make([]Event, 0, len(docs))
	for _, doc := range docs {
		err := o.db.UpdateOne(ctx, collectionName, bson.M{
			"_id":    doc.ID,
			"status": doc.Status,
		}, bson.M{"$set": bson.M{
			"status":     StatusProcessing,
			"updated_at": now,
		}}, database.WriteOptions{})
		if err != nil {
			continue
		}
		doc.Status = StatusProcessing
		doc.UpdatedAt = now
		events = append(events, doc.toEvent())
	}
	return events, nil
}

func (o *MongoOutbox) MarkDone(ctx context.Context, id string) error {
	now := o.now()
	return o.db.UpdateOne(ctx, collectionName, bson.M{"_id": id}, bson.M{"$set": bson.M{
		"status":     StatusDone,
		"updated_at": now,
	}}, database.WriteOptions{})
}

func (o *MongoOutbox) MarkFailed(ctx context.Context, id string, reason string) error {
	now := o.now()
	return o.db.UpdateOne(ctx, collectionName, bson.M{"_id": id}, bson.M{
		"$set": bson.M{
			"status":        StatusFailed,
			"last_error":    reason,
			"process_after": now.Add(time.Minute),
			"updated_at":    now,
		},
		"$inc": bson.M{"retry_count": 1},
	}, database.WriteOptions{})
}

type eventDocument struct {
	ID             string    `bson:"_id" json:"id"`
	IdempotencyKey string    `bson:"idempotency_key" json:"idempotency_key"`
	Type           string    `bson:"type" json:"type"`
	Payload        []byte    `bson:"payload" json:"payload"`
	MaxRetries     int       `bson:"max_retries" json:"max_retries"`
	RetryCount     int       `bson:"retry_count" json:"retry_count"`
	Status         Status    `bson:"status" json:"status"`
	LastError      string    `bson:"last_error,omitempty" json:"last_error,omitempty"`
	ProcessAfter   time.Time `bson:"process_after" json:"process_after"`
	CreatedAt      time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time `bson:"updated_at" json:"updated_at"`
}

func documentFromEvent(event Event) eventDocument {
	return eventDocument(event)
}

func (d eventDocument) toEvent() Event {
	return Event(d)
}

var _ Outbox = (*MongoOutbox)(nil)
