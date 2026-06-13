// Package database defines a thin abstraction over MongoDB that coordinates
// read-through cache, cache invalidation, and distributed locks.
//
// Repository implementations depend on the Database interface and never import
// MongoDB or Redis drivers directly.
package database

import (
	"context"
)

// Database abstracts CRUD operations against a persistent store. The
// implementation (currently MongoDatabase) delegates to MongoDB while the
// CachedDatabase wrapper coordinates cache reads/writes and optional Redis
// locks through typed options.
type Database interface {
	// FindOne retrieves a single document and unmarshals it into dest.
	// May use cache when ReadOptions.CacheKey is set.
	FindOne(ctx context.Context, collection string, filter any, dest any, opts ReadOptions) error
	// FindMany retrieves multiple documents and unmarshals them into dest.
	// Only uses cache when the filter implements CacheableFilter and
	// ReadOptions.CacheKey is set.
	FindMany(ctx context.Context, collection string, filter any, dest any, opts ReadOptions) error
	// InsertOne inserts a single document. Optionally invalidates cache
	// entries through WriteOptions.InvalidateKeys.
	InsertOne(ctx context.Context, collection string, document any, opts WriteOptions) error
	// UpdateOne updates a single document matching filter. Optionally
	// invalidates cache entries through WriteOptions.InvalidateKeys.
	UpdateOne(ctx context.Context, collection string, filter any, update any, opts WriteOptions) error
	// UpdateMany updates all documents matching filter.
	UpdateMany(ctx context.Context, collection string, filter any, update any, opts WriteOptions) error
	// DeleteOne deletes a single document matching filter.
	DeleteOne(ctx context.Context, collection string, filter any, opts WriteOptions) error
	// Count returns the number of documents matching filter.
	Count(ctx context.Context, collection string, filter any) (int64, error)
	// Ping checks connectivity to the database.
	Ping(ctx context.Context) error
	// Close releases any underlying resources.
	Close(ctx context.Context) error
}
