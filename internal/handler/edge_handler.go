package handler

import (
	"github.com/cubetiqlabs/wkr/internal/edge"
	"github.com/gofiber/fiber/v3"
)

type EdgeHandler struct {
	registry *edge.Registry
	router   *edge.Router
}

func NewEdgeHandler(registry *edge.Registry, router *edge.Router) *EdgeHandler {
	return &EdgeHandler{registry: registry, router: router}
}

func (h *EdgeHandler) ListNodes(c fiber.Ctx) error {
	nodes, err := h.registry.ListNodes(c.Context())
	if err != nil {
		return errResponse(c, fiber.StatusInternalServerError, "failed to list nodes")
	}
	return ok(c, nodes)
}

func (h *EdgeHandler) ClusterStatus(c fiber.Ctx) error {
	return ok(c, h.router.EdgeStatus())
}

// SyncWorker receives a worker deployment from another edge node.
func (h *EdgeHandler) SyncWorker(c fiber.Ctx) error {
	secret := c.Get("X-Cubis-Internal-Secret")
	if !h.router.ValidateInternalSecret(secret) {
		return errResponse(c, fiber.StatusUnauthorized, "invalid internal secret")
	}
	// TODO: persist synced worker data
	return ok(c, fiber.Map{"synced": true})
}
