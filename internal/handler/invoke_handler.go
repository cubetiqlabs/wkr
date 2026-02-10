package handler

import (
	"time"

	"github.com/aspect-build/cubis-wkr/internal/model"
	"github.com/aspect-build/cubis-wkr/internal/runtime"
	"github.com/aspect-build/cubis-wkr/internal/service"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type InvokeHandler struct {
	workerService *service.WorkerService
	pool          *runtime.Pool
}

func NewInvokeHandler(workerService *service.WorkerService, pool *runtime.Pool) *InvokeHandler {
	return &InvokeHandler{workerService: workerService, pool: pool}
}

// Invoke executes a worker by name: POST /invoke/:name
func (h *InvokeHandler) Invoke(c fiber.Ctx) error {
	name := c.Params("name")
	if name == "" {
		return errResponse(c, fiber.StatusBadRequest, "worker name required")
	}

	worker, err := h.workerService.GetByName(c.Context(), name)
	if err != nil {
		return errResponse(c, fiber.StatusNotFound, "worker not found")
	}

	// Build headers map
	headers := make(map[string]string)
	c.Request().Header.VisitAll(func(key, value []byte) {
		headers[string(key)] = string(value)
	})

	req := &runtime.ExecutionRequest{
		WorkerName: worker.Name,
		Code:       worker.Code,
		Runtime:    string(worker.Runtime),
		EntryPoint: worker.EntryPoint,
		EnvVars:    worker.EnvVars,
		Payload:    c.Body(),
		Headers:    headers,
		Method:     c.Method(),
		Path:       c.Path(),
	}

	result, err := h.pool.Execute(c.Context(), req)

	// Record invocation asynchronously
	inv := &model.Invocation{
		WorkerID: worker.ID,
	}

	if err != nil {
		inv.StatusCode = 500
		inv.Error = err.Error()
		go h.workerService.RecordInvocation(c.Context(), inv)
		return errResponse(c, fiber.StatusInternalServerError, "worker execution failed: "+err.Error())
	}

	inv.StatusCode = result.StatusCode
	inv.Duration = result.Duration
	inv.MemoryUsed = result.MemoryUsed
	if result.Error != "" {
		inv.Error = result.Error
	}
	go h.workerService.RecordInvocation(c.Context(), inv)

	// Set response headers from worker
	for k, v := range result.Headers {
		c.Set(k, v)
	}
	c.Set("X-Cubis-Worker", worker.Name)
	c.Set("X-Cubis-Duration", result.Duration.String())
	c.Set("X-Cubis-Request-ID", uuid.New().String())
	c.Set("X-Cubis-Timestamp", time.Now().UTC().Format(time.RFC3339))

	return c.Status(result.StatusCode).Send(result.Body)
}
