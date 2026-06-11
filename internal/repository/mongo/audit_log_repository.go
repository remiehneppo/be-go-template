package mongo

import (
	"context"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const auditLogsCollection = "audit_logs"

type AuditLogRepository struct {
	db database.Database
}

func NewAuditLogRepository(db database.Database) *AuditLogRepository {
	return &AuditLogRepository{db: db}
}

func (r *AuditLogRepository) Append(ctx context.Context, event auth.AuditLog) error {
	return r.db.InsertOne(ctx, auditLogsCollection, auditLogDocumentFromDomain(event), database.WriteOptions{})
}

func (r *AuditLogRepository) List(ctx context.Context, filter auth.AuditLogFilter, pagination common.Pagination) ([]auth.AuditLog, error) {
	pagination = pagination.Normalized(20, 100)
	query := bson.M{}
	if filter.ActorUserID != "" {
		query["actor_user_id"] = filter.ActorUserID
	}
	if filter.Action != "" {
		query["action"] = filter.Action
	}
	if filter.RequestID != "" {
		query["request_id"] = filter.RequestID
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

	var docs []auditLogDocument
	if err := r.db.FindMany(ctx, auditLogsCollection, query, &docs, database.ReadOptions{
		Limit:  int64(pagination.Limit),
		Offset: int64(pagination.Offset),
		Sort:   bson.M{"created_at": -1},
	}); err != nil {
		return nil, err
	}
	events := make([]auth.AuditLog, 0, len(docs))
	for _, doc := range docs {
		events = append(events, doc.toDomain())
	}
	return events, nil
}

type auditLogDocument struct {
	ID           string            `bson:"_id" json:"id"`
	ActorUserID  string            `bson:"actor_user_id" json:"actor_user_id"`
	Action       string            `bson:"action" json:"action"`
	ResourceType string            `bson:"resource_type" json:"resource_type"`
	ResourceID   string            `bson:"resource_id" json:"resource_id"`
	IP           string            `bson:"ip" json:"ip"`
	UserAgent    string            `bson:"user_agent" json:"user_agent"`
	RequestID    string            `bson:"request_id" json:"request_id"`
	Metadata     map[string]string `bson:"metadata" json:"metadata"`
	CreatedAt    time.Time         `bson:"created_at" json:"created_at"`
}

func auditLogDocumentFromDomain(event auth.AuditLog) auditLogDocument {
	return auditLogDocument{
		ID:           event.ID,
		ActorUserID:  event.ActorUserID,
		Action:       event.Action,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		IP:           event.IP,
		UserAgent:    event.UserAgent,
		RequestID:    event.RequestID,
		Metadata:     event.Metadata,
		CreatedAt:    event.CreatedAt,
	}
}

func (d auditLogDocument) toDomain() auth.AuditLog {
	return auth.AuditLog{
		ID:           d.ID,
		ActorUserID:  d.ActorUserID,
		Action:       d.Action,
		ResourceType: d.ResourceType,
		ResourceID:   d.ResourceID,
		IP:           d.IP,
		UserAgent:    d.UserAgent,
		RequestID:    d.RequestID,
		Metadata:     d.Metadata,
		CreatedAt:    d.CreatedAt,
	}
}
