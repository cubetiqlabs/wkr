package handler

import (
	"github.com/aspect-build/cubis-wkr/internal/database"
	"github.com/aspect-build/cubis-wkr/internal/runtime"
	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"
)

type HealthHandler struct {
	db   *gorm.DB
	pool *runtime.Pool
}

func NewHealthHandler(db *gorm.DB, pool *runtime.Pool) *HealthHandler {
	return &HealthHandler{db: db, pool: pool}
}

func (h *HealthHandler) Health(c fiber.Ctx) error {
	dbErr := database.HealthCheck(h.db)
	dbStatus := "up"
	if dbErr != nil {
		dbStatus = "down"
	}

	return ok(c, fiber.Map{
		"status":         "ok",
		"database":       dbStatus,
		"active_workers": h.pool.ActiveCount(),
	})
}

func (h *HealthHandler) Ready(c fiber.Ctx) error {
	if err := database.HealthCheck(h.db); err != nil {
		return errResponse(c, fiber.StatusServiceUnavailable, "database not ready")
	}
	return ok(c, fiber.Map{"status": "ready"})
}
