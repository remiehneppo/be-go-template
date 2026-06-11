package database

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var ErrNotFound = errors.New("document not found")

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
	err := d.db.Collection(collection).FindOne(ctx, filter).Decode(dest)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	return err
}

func (d *MongoDatabase) FindMany(ctx context.Context, collection string, filter any, dest any, opts ReadOptions) error {
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
	return cursor.All(ctx, dest)
}

func (d *MongoDatabase) InsertOne(ctx context.Context, collection string, document any, opts WriteOptions) error {
	_, err := d.db.Collection(collection).InsertOne(ctx, document)
	return err
}

func (d *MongoDatabase) UpdateOne(ctx context.Context, collection string, filter any, update any, opts WriteOptions) error {
	result, err := d.db.Collection(collection).UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *MongoDatabase) UpdateMany(ctx context.Context, collection string, filter any, update any, opts WriteOptions) error {
	result, err := d.db.Collection(collection).UpdateMany(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *MongoDatabase) DeleteOne(ctx context.Context, collection string, filter any, opts WriteOptions) error {
	result, err := d.db.Collection(collection).DeleteOne(ctx, filter)
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *MongoDatabase) Close(ctx context.Context) error {
	return d.client.Disconnect(ctx)
}
