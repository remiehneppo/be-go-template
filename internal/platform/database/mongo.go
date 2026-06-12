package database

import (
	"context"
	"errors"
	"strings"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var ErrNotFound = errors.New("document not found")
var ErrConflict = errors.New("document conflict")

type MongoDatabase struct {
	client *mongo.Client
	db     *mongo.Database
}

func NewMongo(client *mongo.Client, databaseName string) *MongoDatabase {
	return &MongoDatabase{
		client: client,
		db:     client.Database(databaseName),
	}
}

func (d *MongoDatabase) FindOne(ctx context.Context, collection string, filter any, dest any, opts ReadOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	err := d.db.Collection(collection).FindOne(ctx, filter).Decode(dest)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	return dependencyError("MongoDatabase.FindOne", err)
}

func (d *MongoDatabase) FindMany(ctx context.Context, collection string, filter any, dest any, opts ReadOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	findOpts := options.Find()
	if opts.Limit > 0 {
		findOpts.SetLimit(opts.Limit)
	}
	if opts.Offset > 0 {
		findOpts.SetSkip(opts.Offset)
	}
	if opts.Sort != nil {
		findOpts.SetSort(opts.Sort)
	}
	cursor, err := d.db.Collection(collection).Find(ctx, filter, findOpts)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	return dependencyError("MongoDatabase.FindMany", cursor.All(ctx, dest))
}

func (d *MongoDatabase) InsertOne(ctx context.Context, collection string, document any, opts WriteOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	_, err := d.db.Collection(collection).InsertOne(ctx, document)
	return dependencyError("MongoDatabase.InsertOne", err)
}

func (d *MongoDatabase) UpdateOne(ctx context.Context, collection string, filter any, update any, opts WriteOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	result, err := d.db.Collection(collection).UpdateOne(ctx, filter, update)
	if err != nil {
		return dependencyError("MongoDatabase.UpdateOne", err)
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *MongoDatabase) UpdateMany(ctx context.Context, collection string, filter any, update any, opts WriteOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	result, err := d.db.Collection(collection).UpdateMany(ctx, filter, update)
	if err != nil {
		return dependencyError("MongoDatabase.UpdateMany", err)
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *MongoDatabase) DeleteOne(ctx context.Context, collection string, filter any, opts WriteOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	result, err := d.db.Collection(collection).DeleteOne(ctx, filter)
	if err != nil {
		return dependencyError("MongoDatabase.DeleteOne", err)
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *MongoDatabase) Count(ctx context.Context, collection string, filter any) (int64, error) {
	count, err := d.db.Collection(collection).CountDocuments(ctx, filter)
	return count, dependencyError("MongoDatabase.Count", err)
}

func (d *MongoDatabase) Ping(ctx context.Context) error {
	return dependencyError("MongoDatabase.Ping", d.client.Ping(ctx, nil))
}

func (d *MongoDatabase) Close(ctx context.Context) error {
	return d.client.Disconnect(ctx)
}

func (d *MongoDatabase) RunInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if fn == nil {
		return nil
	}
	session, err := d.client.StartSession()
	if err != nil {
		return dependencyError("MongoDatabase.RunInTransaction", err)
	}
	defer session.EndSession(context.Background())
	_, err = session.WithTransaction(ctx, func(txCtx context.Context) (any, error) {
		if err := fn(txCtx); err != nil {
			return nil, err
		}
		return nil, nil
	})
	return dependencyError("MongoDatabase.RunInTransaction", err)
}

func IsDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	type codedError interface {
		HasErrorCode(int) bool
	}
	var ce codedError
	if errors.As(err, &ce) && ce.HasErrorCode(11000) {
		return true
	}
	if errors.Is(err, ErrConflict) {
		return true
	}
	if strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
		return true
	}
	return false
}
