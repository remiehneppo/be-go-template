package migration

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const collectionName = "schema_migrations"

type MongoStore struct {
	db *mongo.Database
}

func NewMongoStore(db *mongo.Database) *MongoStore {
	return &MongoStore{db: db}
}

func (s *MongoStore) Ensure(ctx context.Context) error {
	_, err := s.db.Collection(collectionName).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "version", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return err
}

func (s *MongoStore) Has(ctx context.Context, version string) (bool, error) {
	count, err := s.db.Collection(collectionName).CountDocuments(ctx, bson.M{"version": version})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *MongoStore) Record(ctx context.Context, migration AppliedMigration) error {
	_, err := s.db.Collection(collectionName).InsertOne(ctx, bson.M{
		"version":    migration.Version,
		"name":       migration.Name,
		"applied_at": migration.AppliedAt,
	})
	return err
}
