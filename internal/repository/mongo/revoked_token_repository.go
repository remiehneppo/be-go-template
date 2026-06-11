package mongo

import (
	"context"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/auth"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const revokedTokensCollection = "revoked_tokens"

type RevokedTokenRepository struct {
	db database.Database
}

func NewRevokedTokenRepository(db database.Database) *RevokedTokenRepository {
	return &RevokedTokenRepository{db: db}
}

func (r *RevokedTokenRepository) Append(ctx context.Context, token auth.RevokedToken) error {
	return r.db.InsertOne(ctx, revokedTokensCollection, revokedTokenDocumentFromDomain(token), database.WriteOptions{})
}

func (r *RevokedTokenRepository) FindByTokenID(ctx context.Context, tokenID string) (*auth.RevokedToken, error) {
	var doc revokedTokenDocument
	if err := r.db.FindOne(ctx, revokedTokensCollection, bson.M{"_id": tokenID}, &doc, database.ReadOptions{}); err != nil {
		return nil, err
	}
	token := doc.toDomain()
	return &token, nil
}

type revokedTokenDocument struct {
	TokenID   string    `bson:"_id" json:"token_id"`
	UserID    string    `bson:"user_id" json:"user_id"`
	SessionID string    `bson:"session_id" json:"session_id"`
	ExpiresAt time.Time `bson:"expires_at" json:"expires_at"`
	RevokedAt time.Time `bson:"revoked_at" json:"revoked_at"`
}

func revokedTokenDocumentFromDomain(token auth.RevokedToken) revokedTokenDocument {
	return revokedTokenDocument{
		TokenID:   token.TokenID,
		UserID:    token.UserID,
		SessionID: token.SessionID,
		ExpiresAt: token.ExpiresAt,
		RevokedAt: token.RevokedAt,
	}
}

func (d revokedTokenDocument) toDomain() auth.RevokedToken {
	return auth.RevokedToken{
		TokenID:   d.TokenID,
		UserID:    d.UserID,
		SessionID: d.SessionID,
		ExpiresAt: d.ExpiresAt,
		RevokedAt: d.RevokedAt,
	}
}
