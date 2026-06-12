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
	appuser "github.com/remihneppo/be-go-template/internal/app/user"
	"github.com/remihneppo/be-go-template/internal/bootstrap"
	"github.com/remihneppo/be-go-template/internal/config"
	httpserver "github.com/remihneppo/be-go-template/internal/handler/http"
	"github.com/remihneppo/be-go-template/internal/platform/cache"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
	"github.com/remihneppo/be-go-template/internal/platform/health"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
	platformmetrics "github.com/remihneppo/be-go-template/internal/platform/metrics"
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
	apperrors.SetStackTraceEnabled(cfg.Errors.IncludeStack)

	log, closeLog, err := logger.New(logger.Config{
		Level:      cfg.Log.Level,
		Format:     cfg.Log.Format,
		FilePath:   cfg.Log.FilePath,
		ToTerminal: cfg.Log.ToTerminal,
		ToFile:     cfg.Log.ToFile,
		MaxSizeMB:  cfg.Log.MaxSizeMB,
		MaxBackups: cfg.Log.MaxBackups,
		MaxAgeDays: cfg.Log.MaxAgeDays,
		Compress:   cfg.Log.Compress,
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

	authMetrics, err := platformmetrics.NewAuthMetrics(nil, "")
	if err != nil {
		return fmt.Errorf("init auth metrics: %w", err)
	}
	dbMetrics, err := platformmetrics.NewDatabaseMetrics(nil, "")
	if err != nil {
		return fmt.Errorf("init database metrics: %w", err)
	}

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

	redisCache, err := cache.NewRedis(cache.RedisConfig{
		Addr:          cfg.Redis.Addr,
		Password:      cfg.Redis.Password,
		DB:            cfg.Redis.DB,
		LockPrefix:    cfg.Redis.LockPrefix,
		TLSEnabled:    cfg.Redis.TLSEnabled,
		TLSCACert:     cfg.Redis.TLSCACert,
		TLSServerName: cfg.Redis.TLSServerName,
	})
	if err != nil {
		return fmt.Errorf("init redis cache: %w", err)
	}
	defer func() {
		if err := redisCache.Close(); err != nil {
			log.Warn("redis close failed", logger.Any("error", err))
		}
	}()

	baseDB := database.NewMongo(mongoClient, cfg.Mongo.Database)
	db := database.NewCached(baseDB, redisCache, log, dbMetrics)
	userRepo := mongorepo.NewUserRepository(db)
	sessionRepo := mongorepo.NewSessionRepository(db)
	directLoginHistoryRepo := mongorepo.NewLoginHistoryRepository(db)
	directAuditLogRepo := mongorepo.NewAuditLogRepository(db)
	revokedTokenRepo := mongorepo.NewRevokedTokenRepository(db)
	directErrorEventRepo := mongorepo.NewErrorEventRepository(db)
	monitoringStatsRepo := mongorepo.NewMonitoringStatsRepository(db)
	mongoOutbox := platformoutbox.NewMongoOutboxWithConfig(db, cfg.Outbox.DefaultMaxRetries, cfg.Outbox.RetryDelay)
	loginHistoryRepo := appoutbox.NewLoginHistoryRepository(directLoginHistoryRepo, mongoOutbox)
	outboxHandler := appoutbox.NewHandler(directAuditLogRepo, loginHistoryRepo, directErrorEventRepo)
	var outboxDone chan struct{}
	if cfg.Outbox.Enabled {
		outboxDone = make(chan struct{})
		outboxWorker := platformoutbox.NewWorker(mongoOutbox, outboxHandler.Handle, cfg.Outbox.DrainInterval, cfg.Outbox.BatchSize)
		go func() {
			defer close(outboxDone)
			if err := outboxWorker.Run(ctx); err != nil && ctx.Err() == nil {
				log.Error("outbox worker stopped", logger.Any("error", err))
			}
		}()
	} else {
		log.Warn("outbox worker disabled")
	}
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
		Metrics:            authMetrics,
		RefreshTTL:         cfg.JWT.RefreshTTL,
		LockoutMaxFailures: cfg.Auth.LockoutMaxFailures,
		LockoutDuration:    cfg.Auth.LockoutDuration,
	})
	userService := appuser.NewService(userRepo)
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
		UserService:  userService,
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
	if outboxDone != nil {
		select {
		case <-outboxDone:
		case <-time.After(5 * time.Second):
			log.Warn("outbox worker shutdown timed out")
		}
	}
	log.Info("api server stopped")
	return nil
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
	pingCtx, cancel := context.WithTimeout(ctx, cfg.Mongo.ConnectTimeout)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		if disconnectErr := client.Disconnect(context.Background()); disconnectErr != nil {
			fmt.Fprintf(os.Stderr, "warning: disconnect mongo: %v\n", disconnectErr)
		}
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
