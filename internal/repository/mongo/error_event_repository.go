package mongo

import (
	"context"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const errorEventsCollection = "error_events"

type ErrorEventRepository struct {
	db database.Database
}

func NewErrorEventRepository(db database.Database) *ErrorEventRepository {
	return &ErrorEventRepository{db: db}
}

func (r *ErrorEventRepository) Append(ctx context.Context, event auth.ErrorEvent) error {
	return mapWriteError(r.db.InsertOne(ctx, errorEventsCollection, errorEventDocumentFromDomain(event), database.WriteOptions{}))
}

func (r *ErrorEventRepository) List(ctx context.Context, filter auth.ErrorEventFilter, pagination common.Pagination) ([]auth.ErrorEvent, error) {
	pagination = pagination.Normalized(20, 100)
	query := bson.M{}
	if filter.ErrorCode != "" {
		query["error_code"] = filter.ErrorCode
	}
	if filter.RequestID != "" {
		query["request_id"] = filter.RequestID
	}
	if filter.Operation != "" {
		query["operation"] = filter.Operation
	}
	if filter.Status != 0 {
		query["status"] = filter.Status
	}
	if !filter.From.IsZero() || !filter.To.IsZero() {
		createdAt := bson.M{}
		if !filter.From.IsZero() {
			createdAt["$gte"] = filter.From
		}
		if !filter.To.IsZero() {
			createdAt["$lte"] = filter.To
		}
		query["created_at"] = createdAt
	}
	var docs []errorEventDocument
	if err := r.db.FindMany(ctx, errorEventsCollection, query, &docs, database.ReadOptions{
		Limit:  int64(pagination.Limit),
		Offset: int64(pagination.Offset),
		Sort:   bson.M{"created_at": -1},
	}); err != nil {
		return nil, err
	}
	events := make([]auth.ErrorEvent, 0, len(docs))
	for _, doc := range docs {
		events = append(events, doc.toDomain())
	}
	return events, nil
}

type errorEventDocument struct {
	RequestID string    `bson:"request_id" json:"request_id"`
	ErrorCode string    `bson:"error_code" json:"error_code"`
	Operation string    `bson:"operation,omitempty" json:"operation,omitempty"`
	Message   string    `bson:"message" json:"message"`
	Cause     string    `bson:"cause,omitempty" json:"cause,omitempty"`
	Stack     string    `bson:"stack,omitempty" json:"stack,omitempty"`
	Path      string    `bson:"path" json:"path"`
	Method    string    `bson:"method" json:"method"`
	Status    int       `bson:"status" json:"status"`
	UserID    string    `bson:"user_id,omitempty" json:"user_id,omitempty"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

func errorEventDocumentFromDomain(event auth.ErrorEvent) errorEventDocument {
	return errorEventDocument{
		RequestID: event.RequestID,
		ErrorCode: event.ErrorCode,
		Operation: event.Operation,
		Message:   event.Message,
		Cause:     event.Cause,
		Stack:     event.Stack,
		Path:      event.Path,
		Method:    event.Method,
		Status:    event.Status,
		UserID:    event.UserID,
		CreatedAt: event.CreatedAt,
	}
}

func (d errorEventDocument) toDomain() auth.ErrorEvent {
	return auth.ErrorEvent{
		RequestID: d.RequestID,
		ErrorCode: d.ErrorCode,
		Operation: d.Operation,
		Message:   d.Message,
		Cause:     d.Cause,
		Stack:     d.Stack,
		Path:      d.Path,
		Method:    d.Method,
		Status:    d.Status,
		UserID:    d.UserID,
		CreatedAt: d.CreatedAt,
	}
}
