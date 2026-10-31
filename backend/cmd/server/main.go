package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"

	"invoiceflow/internal/api"
	"invoiceflow/internal/config"
	"invoiceflow/internal/logger"
	"invoiceflow/internal/metrics"
	"invoiceflow/internal/monitor"
	"invoiceflow/internal/queue"
	"invoiceflow/internal/secret"
	"invoiceflow/internal/repo"
)

func main() {
	// -------------------------------------------------------------------------
	// Load configuration
	// -------------------------------------------------------------------------
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// -------------------------------------------------------------------------
	// Initialise logger
	// -------------------------------------------------------------------------
	log, err := logger.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialise logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = log.Sync()
	}()

	// -------------------------------------------------------------------------
	// Initialise secret manager (used by JWT, LLM tokens, etc.)
	// -------------------------------------------------------------------------
	secretMgr, err := secret.NewManager(cfg, log)
	if err != nil {
		log.Fatal("secret manager init failed", zap.Error(err))
	}
	// Pre‑load required secrets early so that downstream components can rely on them.
	if err := secretMgr.Warmup(context.Background()); err != nil {
		log.Fatal("secret manager warmup failed", zap.Error(err))
	}

	// -------------------------------------------------------------------------
	// Initialise PostgreSQL connection (GORM)
	// -------------------------------------------------------------------------
	db, err := repo.NewPostgres(cfg, log, secretMgr)
	if err != nil {
		log.Fatal("database connection failed", zap.Error(err))
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("unable to obtain generic DB from GORM", zap.Error(err))
	}
	// Ensure DB is reachable
	if err := sqlDB.Ping(); err != nil {
		log.Fatal("database ping failed", zap.Error(err))
	}
	// Run migrations (if any)
	if err := repo.RunMigrations(db, log); err != nil {
		log.Fatal("database migration failed", zap.Error(err))
	}

	// -------------------------------------------------------------------------
	// Initialise Redis client
	// -------------------------------------------------------------------------
	redisOpts := &redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}
	redisClient := redis.NewClient(redisOpts)
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		log.Fatal("redis ping failed", zap.Error(err))
	}

	// -------------------------------------------------------------------------
	// Initialise Google Cloud Tasks client / wrapper
	// -------------------------------------------------------------------------
	taskQueue, err := queue.NewGoogleTasks(cfg, log)
	if err != nil {
		log.Fatal("cloud tasks client init failed", zap.Error(err))
	}

	// -------------------------------------------------------------------------
	// Register Prometheus metrics
	// -------------------------------------------------------------------------
	metrics.RegisterAll()
	// Expose metrics handler later via router

	// -------------------------------------------------------------------------
	// Start GitHub Actions queue monitor (runs in background)
	// -------------------------------------------------------------------------
	ghMonitor := monitor.NewGitHubActionsMonitor(cfg, log)
	monitorCtx, monitorCancel := context.WithCancel(context.Background())
	defer monitorCancel()
	go ghMonitor.Start(monitorCtx)

	// -------------------------------------------------------------------------
	// Build Gin router with all routes & middlewares
	// -------------------------------------------------------------------------
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := api.NewRouter(api.Dependencies{
		DB:        db,
		Redis:     redisClient,
		Queue:     taskQueue,
		Logger:    log,
		Config:    cfg,
		SecretMgr: secretMgr,
	})

	// Add Prometheus metrics endpoint
	router.GET("/metrics", gin.WrapH(metrics.Handler()))

	// -------------------------------------------------------------------------
	// HTTP server configuration
	// -------------------------------------------------------------------------
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: router,
	}

	// -------------------------------------------------------------------------
	// Run server in a goroutine
	// -------------------------------------------------------------------------
	go func() {
		log.Info("starting HTTP server", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("listen error", zap.Error(err))
		}
	}()

	// -------------------------------------------------------------------------
	// Graceful shutdown handling
	// -------------------------------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("server forced to shutdown", zap.Error(err))
	}

	// Close external resources
	if err := redisClient.Close(); err != nil {
		log.Warn("error closing redis client", zap.Error(err))
	}
	if sqlDB != nil {
		_ = sqlDB.Close()
	}
	if err := taskQueue.Close(); err != nil {
		log.Warn("error closing task queue client", zap.Error(err))
	}
	monitorCancel()

	log.Info("server exiting")
}