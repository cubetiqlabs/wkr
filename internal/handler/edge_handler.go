package handler

import (
	"github.com/cubetiqlabs/wkr/internal/edge"
	"github.com/cubetiqlabs/wkr/internal/logger"
	"github.com/cubetiqlabs/wkr/internal/service"
	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type EdgeHandler struct {
	registry      *edge.Registry
	router        *edge.Router
	workerService *service.WorkerService
}

func NewEdgeHandler(registry *edge.Registry, router *edge.Router, workerService *service.WorkerService) *EdgeHandler {
	return &EdgeHandler{registry: registry, router: router, workerService: workerService}
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

// syncWorkerPayload is the wire format for inter-node worker sync.
type syncWorkerPayload struct {
	Name       string `json:"name"`
	Runtime    string `json:"runtime"`
	EntryPoint string `json:"entry_point"`
	Code       string `json:"code"`
	OwnerID    string `json:"owner_id"`
}

// SyncWorker receives a worker deployment from another edge node.
func (h *EdgeHandler) SyncWorker(c fiber.Ctx) error {
	secret := c.Get("X-Cubis-Internal-Secret")
	if !h.router.ValidateInternalSecret(secret) {
		return errResponse(c, fiber.StatusUnauthorized, "invalid internal secret")
	}

	var payload syncWorkerPayload
	if err := c.Bind().JSON(&payload); err != nil {
		return errResponse(c, fiber.StatusBadRequest, "invalid payload")
	}

	if payload.Name == "" || payload.Code == "" || payload.Runtime == "" {
		return errResponse(c, fiber.StatusBadRequest, "name, code, and runtime are required")
	}

	// Check if worker already exists — update if so, skip create
	existing, _ := h.workerService.GetByName(c.Context(), payload.Name)
	if existing != nil {
		logger.Info("edge sync: worker already exists, skipping",
			zap.String("worker", payload.Name),
		)
		return ok(c, fiber.Map{"synced": true, "action": "exists"})
	}

	logger.Info("edge sync: received worker",
		zap.String("worker", payload.Name),
		zap.String("runtime", payload.Runtime),
	)

	return ok(c, fiber.Map{"synced": true, "action": "received"})
}
