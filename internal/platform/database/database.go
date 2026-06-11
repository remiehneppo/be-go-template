package database

import "context"

type Database interface {
	FindOne(ctx context.Context, collection string, filter any, dest any, opts ReadOptions) error
	FindMany(ctx context.Context, collection string, filter any, dest any, opts ReadOptions) error
	InsertOne(ctx context.Context, collection string, document any, opts WriteOptions) error
	UpdateOne(ctx context.Context, collection string, filter any, update any, opts WriteOptions) error
	UpdateMany(ctx context.Context, collection string, filter any, update any, opts WriteOptions) error
	DeleteOne(ctx context.Context, collection string, filter any, opts WriteOptions) error
	Close(ctx context.Context) error
}
