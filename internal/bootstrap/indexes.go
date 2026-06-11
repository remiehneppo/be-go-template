package bootstrap

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func EnsureIndexes(ctx context.Context, db *mongo.Database) error {
	collections := map[string][]mongo.IndexModel{
		"users": {
			{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		"sessions": {
			{Keys: bson.D{{Key: "user_id", Value: 1}}},
			{Keys: bson.D{{Key: "refresh_token_hash", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "device_id", Value: 1}}},
			{Keys: bson.D{{Key: "revoked_at", Value: 1}}},
			{Keys: bson.D{{Key: "token_family_id", Value: 1}}},
		},
		"login_history": {
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}}},
			{Keys: bson.D{{Key: "email", Value: 1}, {Key: "created_at", Value: -1}}},
			{Keys: bson.D{{Key: "created_at", Value: -1}}},
		},
		"audit_logs": {
			{Keys: bson.D{{Key: "request_id", Value: 1}}},
			{Keys: bson.D{{Key: "action", Value: 1}, {Key: "created_at", Value: -1}}},
			{Keys: bson.D{{Key: "actor_user_id", Value: 1}, {Key: "created_at", Value: -1}}},
			{Keys: bson.D{{Key: "created_at", Value: -1}}},
		},
		"revoked_tokens": {
			{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(int32(time.Second / time.Second))},
		},
		"outbox_events": {
			{Keys: bson.D{{Key: "idempotency_key", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "status", Value: 1}, {Key: "process_after", Value: 1}}},
		},
	}

	for collection, indexes := range collections {
		if _, err := db.Collection(collection).Indexes().CreateMany(ctx, indexes); err != nil {
			return err
		}
	}
	return nil
}
