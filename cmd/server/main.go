package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aspect-build/cubis-wkr/internal/config"
	"github.com/aspect-build/cubis-wkr/internal/database"
	"github.com/aspect-build/cubis-wkr/internal/handler"
	"github.com/aspect-build/cubis-wkr/internal/repository"
	"github.com/aspect-build/cubis-wkr/internal/runtime"
	"github.com/aspect-build/cubis-wkr/internal/server"
	"github.com/aspect-build/cubis-wkr/internal/service"
)

func main() {
	// Load configuration
	cfgPath := os.Getenv("CUBIS_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yml"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Setup structured logging
	setupLogger(cfg.Log)

	slog.Info("starting cubis-wkr",
		"name", cfg.App.Name,
		"version", cfg.App.Version,
		"env", cfg.App.Env,
	)

	// Connect to database
	db, err := database.Connect(cfg.Database)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer database.Close(db)

	// Run migrations
	if cfg.Database.AutoMigrate {
		if err := database.AutoMigrate(db); err != nil {
			slog.Error("database migration failed", "error", err)
			os.Exit(1)
		}
	}

	// Initialize repositories
	userRepo := repository.NewUserRepository(db)
	workerRepo := repository.NewWorkerRepository(db)
	deploymentRepo := repository.NewDeploymentRepository(db)
	invocationRepo := repository.NewInvocationRepository(db)

	// Initialize services
	authService := service.NewAuthService(userRepo, cfg.Auth.JWTSecret, cfg.Auth.JWTExpiry)
	workerService := service.NewWorkerService(workerRepo, deploymentRepo, invocationRepo)

	// Initialize runtime engine and pool
	engine := runtime.NewSandboxEngine()
	pool := runtime.NewPool(engine, cfg.Runtime)

	// Initialize handlers
	authHandler := handler.NewAuthHandler(authService)
	workerHandler := handler.NewWorkerHandler(workerService)
	invokeHandler := handler.NewInvokeHandler(workerService, pool)
	healthHandler := handler.NewHealthHandler(db, pool)

	// Create and configure server
	app := server.New(cfg.Server, cfg.App)
	router := server.NewRouter(app, authHandler, workerHandler, invokeHandler, healthHandler, []byte(cfg.Auth.JWTSecret))
	router.Setup()

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := server.Listen(app, cfg.Server); err != nil {
			slog.Error("server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutdown signal received")

	if err := pool.Shutdown(context.Background()); err != nil {
		slog.Error("runtime pool shutdown error", "error", err)
	}
	if err := server.Shutdown(app); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	slog.Info("cubis-wkr stopped")
}

func setupLogger(cfg config.LogConfig) {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var h slog.Handler
	if cfg.Format == "json" {
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}
	slog.SetDefault(slog.New(h))
}
