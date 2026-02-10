package handler

import (
	"github.com/cubetiqlabs/wkr/internal/database"
	"github.com/cubetiqlabs/wkr/internal/edge"
	"github.com/cubetiqlabs/wkr/internal/runtime"
	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"
)

type HealthHandler struct {
	db       *gorm.DB
	pool     *runtime.Pool
	registry *edge.Registry
}

func NewHealthHandler(db *gorm.DB, pool *runtime.Pool, registry *edge.Registry) *HealthHandler {
	return &HealthHandler{db: db, pool: pool, registry: registry}
}

func (h *HealthHandler) Health(c fiber.Ctx) error {
	dbErr := database.HealthCheck(h.db)
	dbStatus := "up"
	if dbErr != nil {
		dbStatus = "down"
	}

	resp := fiber.Map{
		"status":         "ok",
		"database":       dbStatus,
		"active_workers": h.pool.ActiveCount(),
	}

	if h.registry != nil {
		resp["node_id"] = h.registry.NodeID()
		resp["region"] = h.registry.Region()
	}

	return ok(c, resp)
}

func (h *HealthHandler) Ready(c fiber.Ctx) error {
	if err := database.HealthCheck(h.db); err != nil {
		return errResponse(c, fiber.StatusServiceUnavailable, "database not ready")
	}
	return ok(c, fiber.Map{"status": "ready"})
}
