package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/cubetiqlabs/cubis-wkr/internal/config"
	"github.com/cubetiqlabs/cubis-wkr/internal/database"
	"github.com/cubetiqlabs/cubis-wkr/internal/handler"
	"github.com/cubetiqlabs/cubis-wkr/internal/logger"
	"github.com/cubetiqlabs/cubis-wkr/internal/repository"
	"github.com/cubetiqlabs/cubis-wkr/internal/runtime"
	cubissentry "github.com/cubetiqlabs/cubis-wkr/internal/sentry"
	"github.com/cubetiqlabs/cubis-wkr/internal/server"
	"github.com/cubetiqlabs/cubis-wkr/internal/service"
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

	// Init zap logger
	if err := logger.Init(cfg.Log); err != nil {
		panic("failed to init logger: " + err.Error())
	}
	defer logger.Sync()

	logger.Info("starting cubis-wkr",
		zap.String("name", cfg.App.Name),
		zap.String("version", cfg.App.Version),
		zap.String("env", cfg.App.Env),
	)

	// Init sentry (optional)
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

	// Services
	authService := service.NewAuthService(userRepo, cfg.Auth.JWTSecret, cfg.Auth.JWTExpiry)
	workerService := service.NewWorkerService(workerRepo, deploymentRepo, invocationRepo)

	// Runtime
	engine := runtime.NewSandboxEngine()
	pool := runtime.NewPool(engine, cfg.Runtime)

	// Handlers
	authHandler := handler.NewAuthHandler(authService)
	workerHandler := handler.NewWorkerHandler(workerService)
	invokeHandler := handler.NewInvokeHandler(workerService, pool)
	healthHandler := handler.NewHealthHandler(db, pool)

	// Server
	app := server.New(cfg.Server, cfg.App)
	router := server.NewRouter(app, authHandler, workerHandler, invokeHandler, healthHandler, []byte(cfg.Auth.JWTSecret))
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
