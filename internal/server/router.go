package server

import (
	"time"

	"github.com/cubetiqlabs/wkr/internal/config"
	"github.com/cubetiqlabs/wkr/internal/handler"
	"github.com/cubetiqlabs/wkr/internal/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

type Router struct {
	app           *fiber.App
	authHandler   *handler.AuthHandler
	workerHandler *handler.WorkerHandler
	invokeHandler *handler.InvokeHandler
	healthHandler *handler.HealthHandler
	quotaHandler  *handler.QuotaHandler
	edgeHandler   *handler.EdgeHandler
	jwtSecret     []byte
	metricsCfg    config.MetricsConfig
	corsOrigins   []string
}

func NewRouter(
	app *fiber.App,
	authHandler *handler.AuthHandler,
	workerHandler *handler.WorkerHandler,
	invokeHandler *handler.InvokeHandler,
	healthHandler *handler.HealthHandler,
	quotaHandler *handler.QuotaHandler,
	edgeHandler *handler.EdgeHandler,
	jwtSecret []byte,
	metricsCfg config.MetricsConfig,
	corsOrigins []string,
) *Router {
	return &Router{
		app:           app,
		authHandler:   authHandler,
		workerHandler: workerHandler,
		invokeHandler: invokeHandler,
		healthHandler: healthHandler,
		quotaHandler:  quotaHandler,
		edgeHandler:   edgeHandler,
		jwtSecret:     jwtSecret,
		metricsCfg:    metricsCfg,
		corsOrigins:   corsOrigins,
	}
}

func (r *Router) Setup() *middleware.RateLimiter {
	r.app.Use(recover.New())
	r.app.Use(middleware.SecurityHeaders())
	r.app.Use(middleware.RequestLogger())
	if r.metricsCfg.Enabled {
		r.app.Use(middleware.PrometheusMiddleware())
	}
	r.app.Use(cors.New(cors.Config{
		AllowOrigins: r.corsOrigins,
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-API-Key"},
		MaxAge:       int(12 * time.Hour / time.Second),
	}))

	limiter := middleware.NewRateLimiter(60, time.Minute)

	// Health & metrics
	r.app.Get("/health", r.healthHandler.Health)
	r.app.Get("/ready", r.healthHandler.Ready)
	if r.metricsCfg.Enabled {
		r.app.Get(r.metricsCfg.Path, func(c fiber.Ctx) error {
			fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler())(c.RequestCtx())
			return nil
		})
	}

	v1 := r.app.Group("/api/v1")

	// Auth (public, rate-limited)
	auth := v1.Group("/auth")
	auth.Use(limiter.Handler())
	auth.Post("/register", r.authHandler.Register)
	auth.Post("/login", r.authHandler.Login)

	// Workers (authenticated)
	workers := v1.Group("/workers")
	workers.Use(middleware.Auth(r.jwtSecret))
	workers.Post("/", r.workerHandler.Create)
	workers.Get("/", r.workerHandler.List)
	workers.Put("/by-name/:name", r.workerHandler.UpdateByName)
	workers.Delete("/by-name/:name", r.workerHandler.DeleteByName)
	workers.Get("/:id", r.workerHandler.Get)
	workers.Put("/:id", r.workerHandler.Update)
	workers.Delete("/:id", r.workerHandler.Delete)

	// Account quota & usage (authenticated)
	account := v1.Group("/account")
	account.Use(middleware.Auth(r.jwtSecret))
	account.Get("/usage", r.quotaHandler.GetQuota)

	// Edge cluster (authenticated, admin)
	edgeGroup := v1.Group("/edge")
	edgeGroup.Use(middleware.Auth(r.jwtSecret))
	edgeGroup.Get("/nodes", r.edgeHandler.ListNodes)
	edgeGroup.Get("/status", r.edgeHandler.ClusterStatus)

	// Invoke (public, rate-limited)
	invoke := v1.Group("/invoke")
	invoke.Use(limiter.Handler())
	invoke.Post("/:name", r.invokeHandler.Invoke)
	invoke.Get("/:name", r.invokeHandler.Invoke)

	// Internal edge sync (secret-authenticated, not public)
	r.app.Post("/internal/sync/worker", r.edgeHandler.SyncWorker)

	return limiter
}
