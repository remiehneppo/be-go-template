package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	appauth "github.com/remihneppo/be-go-template/internal/app/auth"
	appmonitoring "github.com/remihneppo/be-go-template/internal/app/monitoring"
	appoutbox "github.com/remihneppo/be-go-template/internal/app/outbox"
	"github.com/remihneppo/be-go-template/internal/bootstrap"
	"github.com/remihneppo/be-go-template/internal/config"
	httpserver "github.com/remihneppo/be-go-template/internal/handler/http"
	"github.com/remihneppo/be-go-template/internal/platform/cache"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	"github.com/remihneppo/be-go-template/internal/platform/health"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
	platformoutbox "github.com/remihneppo/be-go-template/internal/platform/outbox"
	"github.com/remihneppo/be-go-template/internal/platform/ratelimit"
	mongorepo "github.com/remihneppo/be-go-template/internal/repository/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "api failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log, closeLog, err := logger.New(logger.Config{
		Level:      cfg.Log.Level,
		Format:     cfg.Log.Format,
		FilePath:   cfg.Log.FilePath,
		ToTerminal: cfg.Log.ToTerminal,
		ToFile:     cfg.Log.ToFile,
	})
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer func() {
		if err := closeLog(); err != nil {
			fmt.Fprintf(os.Stderr, "close logger: %v\n", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mongoClient, err := connectMongo(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mongoClient.Disconnect(shutdownCtx); err != nil {
			log.Warn("mongo disconnect failed", logger.Any("error", err))
		}
	}()

	mongoDatabase := mongoClient.Database(cfg.Mongo.Database)
	if err := bootstrap.EnsureIndexes(ctx, mongoDatabase); err != nil {
		return fmt.Errorf("ensure indexes: %w", err)
	}

	redisCache := cache.NewRedis(cache.RedisConfig{
		Addr:          cfg.Redis.Addr,
		Password:      cfg.Redis.Password,
		DB:            cfg.Redis.DB,
		LockPrefix:    cfg.Redis.LockPrefix,
		TLSEnabled:    cfg.Redis.TLSEnabled,
		TLSServerName: cfg.Redis.TLSServerName,
	})
	defer func() {
		if err := redisCache.Close(); err != nil {
			log.Warn("redis close failed", logger.Any("error", err))
		}
	}()

	baseDB := database.NewMongo(mongoClient, cfg.Mongo.Database)
	db := database.NewCached(baseDB, redisCache, log)
	userRepo := mongorepo.NewUserRepository(db)
	sessionRepo := mongorepo.NewSessionRepository(db)
	loginHistoryRepo := mongorepo.NewLoginHistoryRepository(db)
	directAuditLogRepo := mongorepo.NewAuditLogRepository(db)
	revokedTokenRepo := mongorepo.NewRevokedTokenRepository(db)
	directErrorEventRepo := mongorepo.NewErrorEventRepository(db)
	monitoringStatsRepo := mongorepo.NewMonitoringStatsRepository(db)
	mongoOutbox := platformoutbox.NewMongoOutbox(db)
	outboxHandler := appoutbox.NewHandler(directAuditLogRepo, directErrorEventRepo)
	outboxWorker := platformoutbox.NewWorker(mongoOutbox, outboxHandler.Handle, 5*time.Second, 10)
	go func() {
		if err := outboxWorker.Run(ctx); err != nil && ctx.Err() == nil {
			log.Error("outbox worker stopped", logger.Any("error", err))
		}
	}()
	auditLogRepo := appoutbox.NewAuditLogRepository(directAuditLogRepo, mongoOutbox)
	errorEventRepo := appoutbox.NewErrorEventRepository(directErrorEventRepo, mongoOutbox)

	tokenService, err := appauth.NewTokenService(appauth.TokenConfig{
		CurrentKey:       cfg.JWT.AccessCurrentKey,
		PreviousKey:      cfg.JWT.AccessPreviousKey,
		PreviousNotAfter: cfg.JWT.PreviousNotAfter,
		AccessTTL:        cfg.JWT.AccessTTL,
		RefreshTTL:       cfg.JWT.RefreshTTL,
	}, redisCache, revokedTokenRepo)
	if err != nil {
		return fmt.Errorf("init token service: %w", err)
	}
	authService := appauth.NewService(appauth.ServiceDependencies{
		Users:              userRepo,
		Sessions:           sessionRepo,
		LoginHistory:       loginHistoryRepo,
		AuditLogs:          auditLogRepo,
		RevokedTokens:      revokedTokenRepo,
		Tokens:             tokenService,
		RefreshTTL:         cfg.JWT.RefreshTTL,
		LockoutMaxFailures: cfg.Auth.LockoutMaxFailures,
		LockoutDuration:    cfg.Auth.LockoutDuration,
	})
	readiness := health.NewReadinessChecker(db, redisCache, health.ReadinessConfig{
		Timeout:                cfg.Readiness.Timeout,
		RequiresRedis:          cfg.Readiness.RequiresRedis,
		MongoDegradedThreshold: cfg.Readiness.MongoDegradedThreshold,
		RedisDegradedThreshold: cfg.Readiness.RedisDegradedThreshold,
	})
	monitoringService := appmonitoring.NewService(appmonitoring.Dependencies{
		ServiceName:       cfg.App.Name,
		Version:           cfg.App.Env,
		StartedAt:         time.Now().UTC(),
		DependencyChecker: readiness,
		AuthStats:         monitoringStatsRepo,
		AuditLogs:         auditLogRepo,
		ErrorEvents:       errorEventRepo,
	})
	router := httpserver.NewRouterWithDependencies(cfg, log, httpserver.RouterDependencies{
		AuthService:  authService,
		TokenService: tokenService,
		Monitoring:   monitoringService,
		ErrorEvents:  errorEventRepo,
		RateLimiter:  ratelimit.NewRedisLimiter(redisCache),
		Readiness:    readiness,
	})

	server := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("api server starting", logger.String("app", cfg.App.Name), logger.String("env", cfg.App.Env), logger.String("addr", cfg.HTTP.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		log.Info("api shutdown requested")
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("serve http: %w", err)
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}
	log.Info("api server stopped")
	return nil
}

func connectMongo(ctx context.Context, cfg config.Config) (*mongo.Client, error) {
	clientOptions := options.Client().
		ApplyURI(cfg.Mongo.URI).
		SetMaxPoolSize(cfg.Mongo.MaxPoolSize).
		SetMinPoolSize(cfg.Mongo.MinPoolSize).
		SetConnectTimeout(cfg.Mongo.ConnectTimeout).
		SetReadPreference(readPreference(cfg.Mongo.ReadPreference))
	client, err := mongo.Connect(clientOptions)
	if err != nil {
		return nil, fmt.Errorf("connect mongo: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, cfg.Mongo.ConnectTimeout)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping mongo: %w", err)
	}
	return client, nil
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
