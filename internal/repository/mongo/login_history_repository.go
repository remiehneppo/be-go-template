package mongo

import (
	"context"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/domain/common"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const loginHistoryCollection = "login_history"

type LoginHistoryRepository struct {
	db database.Database
}

func NewLoginHistoryRepository(db database.Database) *LoginHistoryRepository {
	return &LoginHistoryRepository{db: db}
}

func (r *LoginHistoryRepository) Append(ctx context.Context, event auth.LoginHistory) error {
	return r.db.InsertOne(ctx, loginHistoryCollection, loginHistoryDocumentFromDomain(event), database.WriteOptions{})
}

func (r *LoginHistoryRepository) ListByUserID(ctx context.Context, userID string, pagination common.Pagination) ([]auth.LoginHistory, error) {
	pagination = pagination.Normalized(20, 100)
	var docs []loginHistoryDocument
	if err := r.db.FindMany(ctx, loginHistoryCollection, bson.M{"user_id": userID}, &docs, database.ReadOptions{
		Limit:  int64(pagination.Limit),
		Offset: int64(pagination.Offset),
		Sort:   bson.M{"created_at": -1},
	}); err != nil {
		return nil, err
	}
	events := make([]auth.LoginHistory, 0, len(docs))
	for _, doc := range docs {
		events = append(events, doc.toDomain())
	}
	return events, nil
}

type loginHistoryDocument struct {
	ID            string    `bson:"_id" json:"id"`
	UserID        string    `bson:"user_id" json:"user_id"`
	Email         string    `bson:"email" json:"email"`
	Success       bool      `bson:"success" json:"success"`
	FailureReason string    `bson:"failure_reason,omitempty" json:"failure_reason,omitempty"`
	IP            string    `bson:"ip" json:"ip"`
	UserAgent     string    `bson:"user_agent" json:"user_agent"`
	DeviceID      string    `bson:"device_id" json:"device_id"`
	CreatedAt     time.Time `bson:"created_at" json:"created_at"`
}

func loginHistoryDocumentFromDomain(event auth.LoginHistory) loginHistoryDocument {
	return loginHistoryDocument{
		ID:            event.ID,
		UserID:        event.UserID,
		Email:         event.Email,
		Success:       event.Success,
		FailureReason: event.FailureReason,
		IP:            event.IP,
		UserAgent:     event.UserAgent,
		DeviceID:      auth.NormalizeDeviceID(event.DeviceID),
		CreatedAt:     event.CreatedAt,
	}
}

func (d loginHistoryDocument) toDomain() auth.LoginHistory {
	return auth.LoginHistory{
		ID:            d.ID,
		UserID:        d.UserID,
		Email:         d.Email,
		Success:       d.Success,
		FailureReason: d.FailureReason,
		IP:            d.IP,
		UserAgent:     d.UserAgent,
		DeviceID:      d.DeviceID,
		CreatedAt:     d.CreatedAt,
	}
}
