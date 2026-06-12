package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	appauth "github.com/remihneppo/be-go-template/internal/app/auth"
	appseed "github.com/remihneppo/be-go-template/internal/app/seed"
	"github.com/remihneppo/be-go-template/internal/bootstrap"
	"github.com/remihneppo/be-go-template/internal/config"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	"github.com/remihneppo/be-go-template/internal/repository/mongo"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "seed failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	input := appseed.AdminInput{
		Email:    strings.TrimSpace(os.Getenv("ADMIN_EMAIL")),
		Password: os.Getenv("ADMIN_PASSWORD"),
		Name:     strings.TrimSpace(os.Getenv("ADMIN_NAME")),
	}
	if input.Email == "" {
		return fmt.Errorf("ADMIN_EMAIL is required")
	}
	if strings.TrimSpace(input.Password) == "" {
		return fmt.Errorf("ADMIN_PASSWORD is required")
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
	mongoDB := client.Database(cfg.Mongo.Database)
	if err := bootstrap.EnsureIndexes(ctx, mongoDB); err != nil {
		return fmt.Errorf("ensure indexes: %w", err)
	}
	userRepo := mongo.NewUserRepository(database.NewMongo(client, cfg.Mongo.Database))
	result, err := appseed.NewAdminSeeder(appseed.AdminSeederDependencies{
		Users:     userRepo,
		Passwords: appauth.BcryptHasher{Cost: cfg.Auth.BcryptCost},
	}).SeedAdmin(ctx, input)
	if err != nil {
		return err
	}
	switch {
	case result.Created:
		fmt.Printf("admin user created: %s\n", result.Email)
	case result.Updated:
		fmt.Printf("admin role granted: %s\n", result.Email)
	default:
		fmt.Printf("admin user already present: %s\n", result.Email)
	}
	return nil
}

func connectMongo(ctx context.Context, cfg config.Config) (*mongodriver.Client, error) {
	clientOptions := options.Client().
		ApplyURI(cfg.Mongo.URI).
		SetMaxPoolSize(uint64(cfg.Mongo.MaxPoolSize)).
		SetMinPoolSize(uint64(cfg.Mongo.MinPoolSize)).
		SetConnectTimeout(cfg.Mongo.ConnectTimeout).
		SetReadPreference(readPreference(cfg.Mongo.ReadPreference))
	client, err := mongodriver.Connect(clientOptions)
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
