package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cubetiqlabs/wkr/internal/config"
	"github.com/cubetiqlabs/wkr/internal/database"
	"github.com/cubetiqlabs/wkr/internal/edge"
	"github.com/cubetiqlabs/wkr/internal/handler"
	"github.com/cubetiqlabs/wkr/internal/logger"
	"github.com/cubetiqlabs/wkr/internal/metrics"
	"github.com/cubetiqlabs/wkr/internal/repository"
	"github.com/cubetiqlabs/wkr/internal/runtime"
	"github.com/cubetiqlabs/wkr/internal/security"
	cubissentry "github.com/cubetiqlabs/wkr/internal/sentry"
	"github.com/cubetiqlabs/wkr/internal/server"
	"github.com/cubetiqlabs/wkr/internal/service"
	"go.uber.org/zap"
)

func main() {
	var cfgPath string
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	if cfgPath == "" {
		cfgPath = os.Getenv("CUBIS_CONFIG")
		if cfgPath == "" {
			cfgPath = "config.yml"
		}
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
		zap.String("node_id", cfg.Edge.NodeID),
		zap.String("region", cfg.Edge.Region),
		zap.String("role", cfg.Edge.Role),
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
	workerService := service.NewWorkerService(workerRepo, deploymentRepo, invocationRepo, userRepo, cfg.Runtime.EncryptionKey)
	quotaService := service.NewQuotaService(quotaRepo, workerRepo)

	// Security
	validator := security.NewValidator(cfg.Security)
	auditor := security.NewAuditor(db, cfg.Security.AuditLog, cfg.Edge.NodeID)

	// Runtime
	engine := runtime.NewSandboxEngine(cfg.Edge.NodeID)
	pool := runtime.NewPool(engine, cfg.Runtime, validator, cfg.Edge.NodeID, cfg.Edge.Region)

	// Edge cluster
	registry := edge.NewRegistry(db, cfg.Edge)
	edgeRouter := edge.NewRouter(registry, cfg.Edge.InternalSecret)

	// Wire pool → registry so active worker count is visible to other nodes
	pool.SetActiveCallback(func(count int) {
		registry.UpdateActiveWorkers(count)
	})

	endpoint := cfg.Edge.AdvertiseAddr
	if endpoint == "" {
		host := cfg.Server.Host
		if host == "" || host == "0.0.0.0" || host == "::" {
			// Resolve actual hostname so other nodes can reach us
			if h, err := os.Hostname(); err == nil && h != "" {
				host = h
			} else {
				host = "127.0.0.1"
			}
		}
		endpoint = fmt.Sprintf("http://%s:%d", host, cfg.Server.Port)
	}
	logger.Info("registering edge node",
		zap.String("node_id", cfg.Edge.NodeID),
		zap.String("role", cfg.Edge.Role),
		zap.String("endpoint", endpoint),
	)
	if err := registry.RegisterSelf(endpoint, cfg.Runtime.MaxConcurrentWorkers); err != nil {
		logger.Error("edge registration failed", zap.Error(err))
	}
	registry.StartHeartbeat()

	// Handlers
	authHandler := handler.NewAuthHandler(authService)
	workerHandler := handler.NewWorkerHandler(workerService, quotaService)
	invokeHandler := handler.NewInvokeHandler(workerService, quotaService, pool, auditor, edgeRouter, cfg.Edge.NodeID)
	healthHandler := handler.NewHealthHandler(db, pool, registry)
	quotaHandler := handler.NewQuotaHandler(quotaService)
	edgeHandler := handler.NewEdgeHandler(registry, edgeRouter, workerService)

	// Server
	app := server.New(cfg.Server, cfg.App)
	router := server.NewRouter(app, authHandler, workerHandler, invokeHandler, healthHandler, quotaHandler, edgeHandler, []byte(cfg.Auth.JWTSecret), cfg.Metrics, cfg.Server.CORSOrigins)
	limiter := router.Setup()

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Uptime tracker
	startTime := time.Now()
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				metrics.UptimeSeconds.Set(time.Since(startTime).Seconds())
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		if err := server.Listen(app, cfg.Server); err != nil {
			logger.Error("server error", zap.Error(err))
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	limiter.Stop()
	registry.Shutdown()
	if err := pool.Shutdown(context.Background()); err != nil {
		logger.Error("runtime pool shutdown error", zap.Error(err))
	}
	if err := server.Shutdown(app); err != nil {
		logger.Error("server shutdown error", zap.Error(err))
	}

	logger.Info("cubis-wkr stopped")
}
