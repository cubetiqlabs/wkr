package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/cubetiqlabs/wkr/internal/config"
	"github.com/cubetiqlabs/wkr/internal/database"
	"github.com/cubetiqlabs/wkr/internal/handler"
	"github.com/cubetiqlabs/wkr/internal/logger"
	"github.com/cubetiqlabs/wkr/internal/repository"
	"github.com/cubetiqlabs/wkr/internal/runtime"
	cubissentry "github.com/cubetiqlabs/wkr/internal/sentry"
	"github.com/cubetiqlabs/wkr/internal/server"
	"github.com/cubetiqlabs/wkr/internal/service"
	"go.uber.org/zap"
)

func main() {
	cfgPath := os.Getenv("CUBIS_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yml"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	if err := logger.Init(cfg.Log); err != nil {
		panic("failed to init logger: " + err.Error())
	}
	defer logger.Sync()

	logger.Info("starting cubis-wkr",
		zap.String("name", cfg.App.Name),
		zap.String("version", cfg.App.Version),
		zap.String("env", cfg.App.Env),
	)

	if err := cubissentry.Init(cfg.Sentry); err != nil {
		logger.Error("sentry init failed", zap.Error(err))
	}
	defer cubissentry.Flush()

	// Database
	db, err := database.Connect(cfg.Database)
	if err != nil {
		logger.Fatal("database connection failed", zap.Error(err))
	}
	defer database.Close(db)

	if cfg.Database.AutoMigrate {
		if err := database.AutoMigrate(db); err != nil {
			logger.Fatal("database migration failed", zap.Error(err))
		}
	}

	// Repositories
	userRepo := repository.NewUserRepository(db)
	workerRepo := repository.NewWorkerRepository(db)
	deploymentRepo := repository.NewDeploymentRepository(db)
	invocationRepo := repository.NewInvocationRepository(db)
	quotaRepo := repository.NewQuotaRepository(db)

	// Services
	authService := service.NewAuthService(userRepo, cfg.Auth.JWTSecret, cfg.Auth.JWTExpiry)
	workerService := service.NewWorkerService(workerRepo, deploymentRepo, invocationRepo, cfg.Runtime.EncryptionKey)
	quotaService := service.NewQuotaService(quotaRepo, workerRepo)

	// Runtime
	engine := runtime.NewSandboxEngine()
	pool := runtime.NewPool(engine, cfg.Runtime)

	// Handlers
	authHandler := handler.NewAuthHandler(authService)
	workerHandler := handler.NewWorkerHandler(workerService, quotaService)
	invokeHandler := handler.NewInvokeHandler(workerService, quotaService, pool)
	healthHandler := handler.NewHealthHandler(db, pool)
	quotaHandler := handler.NewQuotaHandler(quotaService)

	// Server
	app := server.New(cfg.Server, cfg.App)
	router := server.NewRouter(app, authHandler, workerHandler, invokeHandler, healthHandler, quotaHandler, []byte(cfg.Auth.JWTSecret))
	router.Setup()

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := server.Listen(app, cfg.Server); err != nil {
			logger.Error("server error", zap.Error(err))
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	if err := pool.Shutdown(context.Background()); err != nil {
		logger.Error("runtime pool shutdown error", zap.Error(err))
	}
	if err := server.Shutdown(app); err != nil {
		logger.Error("server shutdown error", zap.Error(err))
	}

	logger.Info("cubis-wkr stopped")
}
