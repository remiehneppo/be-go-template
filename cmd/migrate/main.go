package main

import (
	"context"
	"fmt"
	"os"

	"github.com/remihneppo/be-go-template/internal/bootstrap"
	"github.com/remihneppo/be-go-template/internal/config"
	"github.com/remihneppo/be-go-template/internal/platform/migration"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "migrate failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Mongo.ConnectTimeout)
	defer cancel()
	client, err := connectMongo(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "warning: disconnect mongo: %v\n", err)
		}
	}()
	db := client.Database(cfg.Mongo.Database)
	runner := migration.NewRunner(migration.NewMongoStore(db))
	applied, err := runner.Run(ctx, migrations(db))
	if err != nil {
		return err
	}
	fmt.Printf("migrations applied: %d\n", len(applied))
	for _, item := range applied {
		fmt.Printf("- %s %s\n", item.Version, item.Name)
	}
	return nil
}

func migrations(db *mongo.Database) []migration.Migration {
	return []migration.Migration{
		{
			Version: "202606110001",
			Name:    "bootstrap_indexes",
			Apply: func(ctx context.Context) error {
				return bootstrap.EnsureIndexes(ctx, db)
			},
		},
	}
}

func connectMongo(ctx context.Context, cfg config.Config) (*mongo.Client, error) {
	clientOptions := options.Client().
		ApplyURI(cfg.Mongo.URI).
		SetMaxPoolSize(uint64(cfg.Mongo.MaxPoolSize)).
		SetMinPoolSize(uint64(cfg.Mongo.MinPoolSize)).
		SetConnectTimeout(cfg.Mongo.ConnectTimeout).
		SetReadPreference(readPreference(cfg.Mongo.ReadPreference))
	client, err := mongo.Connect(clientOptions)
	if err != nil {
		return nil, fmt.Errorf("connect mongo: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		if err := client.Disconnect(context.Background()); err != nil {
			logDisconnectFailure(err)
		}
		return nil, fmt.Errorf("ping mongo: %w", err)
	}
	return client, nil
}

func logDisconnectFailure(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: disconnect mongo: %v\n", err)
	}
}

func readPreference(value string) *readpref.ReadPref {
	switch value {
	case "primaryPreferred":
		return readpref.PrimaryPreferred()
	case "secondary":
		return readpref.Secondary()
	case "secondaryPreferred":
		return readpref.SecondaryPreferred()
	case "nearest":
		return readpref.Nearest()
	default:
		return readpref.Primary()
	}
}
