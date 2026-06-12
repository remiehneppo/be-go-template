package main

import (
	"context"
	"fmt"
	"io"
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
	return runMigrations(context.Background(), cfg, os.Stdout, buildMigrationRunner)
}

type migrationRunner interface {
	Run(ctx context.Context, migrations []migration.Migration) ([]migration.AppliedMigration, error)
}

type migrationRunnerHandle struct {
	Runner   migrationRunner
	Database func() *mongo.Database
	Close    func() error
}

type migrationRunnerBuilder func(ctx context.Context, cfg config.Config) (*migrationRunnerHandle, error)

func runMigrations(ctx context.Context, cfg config.Config, out io.Writer, build migrationRunnerBuilder) error {
	ctx, cancel := context.WithTimeout(ctx, cfg.Mongo.ConnectTimeout)
	defer cancel()

	handle, err := build(ctx, cfg)
	if err != nil {
		return err
	}
	if handle == nil || handle.Runner == nil {
		return fmt.Errorf("migration runner is required")
	}
	if handle.Close != nil {
		defer func() {
			if err := handle.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "warning: disconnect mongo: %v\n", err)
			}
		}()
	}
	if handle.Database == nil {
		return fmt.Errorf("migration database is required")
	}
	applied, err := handle.Runner.Run(ctx, migrations(handle.Database()))
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "migrations applied: %d\n", len(applied))
	for _, item := range applied {
		fmt.Fprintf(out, "- %s %s\n", item.Version, item.Name)
	}
	return nil
}

func buildMigrationRunner(ctx context.Context, cfg config.Config) (*migrationRunnerHandle, error) {
	client, err := connectMongo(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &migrationRunnerHandle{
		Runner: migration.NewRunner(migration.NewMongoStore(client.Database(cfg.Mongo.Database))),
		Database: func() *mongo.Database {
			return client.Database(cfg.Mongo.Database)
		},
		Close: func() error {
			return client.Disconnect(context.Background())
		},
	}, nil
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
