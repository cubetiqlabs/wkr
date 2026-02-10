package server

import (
	"time"

	"github.com/cubetiqlabs/wkr/internal/handler"
	"github.com/cubetiqlabs/wkr/internal/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/recover"
)

type Router struct {
	app           *fiber.App
	authHandler   *handler.AuthHandler
	workerHandler *handler.WorkerHandler
	invokeHandler *handler.InvokeHandler
	healthHandler *handler.HealthHandler
	quotaHandler  *handler.QuotaHandler
	jwtSecret     []byte
}

func NewRouter(
	app *fiber.App,
	authHandler *handler.AuthHandler,
	workerHandler *handler.WorkerHandler,
	invokeHandler *handler.InvokeHandler,
	healthHandler *handler.HealthHandler,
	quotaHandler *handler.QuotaHandler,
	jwtSecret []byte,
) *Router {
	return &Router{
		app:           app,
		authHandler:   authHandler,
		workerHandler: workerHandler,
		invokeHandler: invokeHandler,
		healthHandler: healthHandler,
		quotaHandler:  quotaHandler,
		jwtSecret:     jwtSecret,
	}
}

func (r *Router) Setup() {
	r.app.Use(recover.New())
	r.app.Use(middleware.SecurityHeaders())
	r.app.Use(middleware.RequestLogger())
	r.app.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-API-Key"},
		MaxAge:       int(12 * time.Hour / time.Second),
	}))

	limiter := middleware.NewRateLimiter(60, time.Minute)

	r.app.Get("/health", r.healthHandler.Health)
	r.app.Get("/ready", r.healthHandler.Ready)

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
	workers.Get("/:id", r.workerHandler.Get)
	workers.Put("/:id", r.workerHandler.Update)
	workers.Delete("/:id", r.workerHandler.Delete)

	// Account quota & usage (authenticated)
	account := v1.Group("/account")
	account.Use(middleware.Auth(r.jwtSecret))
	account.Get("/usage", r.quotaHandler.GetQuota)

	// Invoke (public, rate-limited)
	invoke := v1.Group("/invoke")
	invoke.Use(limiter.Handler())
	invoke.Post("/:name", r.invokeHandler.Invoke)
	invoke.Get("/:name", r.invokeHandler.Invoke)
}
