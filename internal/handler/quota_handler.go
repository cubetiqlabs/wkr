package handler

import (
	"github.com/cubetiqlabs/wkr/internal/middleware"
	"github.com/cubetiqlabs/wkr/internal/service"
	"github.com/gofiber/fiber/v3"
)

type QuotaHandler struct {
	quotaService *service.QuotaService
}

func NewQuotaHandler(quotaService *service.QuotaService) *QuotaHandler {
	return &QuotaHandler{quotaService: quotaService}
}

// GetQuota returns the user's account quota and current usage.
func (h *QuotaHandler) GetQuota(c fiber.Ctx) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return errResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	quota, err := h.quotaService.GetQuota(c.Context(), userID)
	if err != nil {
		return errResponse(c, fiber.StatusInternalServerError, "failed to get quota")
	}

	usage, _ := h.quotaService.GetUsage(c.Context(), userID)

	return ok(c, fiber.Map{
		"quota": quota,
		"usage": usage,
	})
}
