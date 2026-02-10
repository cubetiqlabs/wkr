package handler

import (
	"context"
	"time"

	"github.com/cubetiqlabs/wkr/internal/logger"
	"github.com/cubetiqlabs/wkr/internal/model"
	"github.com/cubetiqlabs/wkr/internal/runtime"
	"github.com/cubetiqlabs/wkr/internal/service"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type InvokeHandler struct {
	workerService *service.WorkerService
	quotaService  *service.QuotaService
	pool          *runtime.Pool
}

func NewInvokeHandler(workerService *service.WorkerService, quotaService *service.QuotaService, pool *runtime.Pool) *InvokeHandler {
	return &InvokeHandler{workerService: workerService, quotaService: quotaService, pool: pool}
}

func (h *InvokeHandler) Invoke(c fiber.Ctx) error {
	name := c.Params("name")
	if name == "" {
		return errResponse(c, fiber.StatusBadRequest, "worker name required")
	}

	worker, err := h.workerService.GetByName(c.Context(), name)
	if err != nil {
		return errResponse(c, fiber.StatusNotFound, "worker not found")
	}

	// Enforce account quota
	if err := h.quotaService.CheckInvocationAllowed(c.Context(), worker.OwnerID); err != nil {
		switch err {
		case service.ErrQuotaRequestsExceeded:
			return errResponse(c, fiber.StatusTooManyRequests, "daily request limit exceeded")
		case service.ErrQuotaExecTimeExceeded:
			return errResponse(c, fiber.StatusTooManyRequests, "daily execution time limit exceeded")
		case service.ErrQuotaBandwidthExceeded:
			return errResponse(c, fiber.StatusTooManyRequests, "daily bandwidth limit exceeded")
		default:
			logger.Error("quota check failed", zap.Error(err))
		}
	}

	// Build headers map
	headers := make(map[string]string)
	c.Request().Header.VisitAll(func(key, value []byte) {
		headers[string(key)] = string(value)
	})

	reqBody := c.Body()
	requestBytes := int64(len(reqBody))

	req := &runtime.ExecutionRequest{
		WorkerName: worker.Name,
		Code:       worker.Code,
		CodeHash:   worker.CodeHash,
		Runtime:    string(worker.Runtime),
		EntryPoint: worker.EntryPoint,
		EnvVars:    worker.EnvVars,
		Payload:    reqBody,
		Headers:    headers,
		Method:     c.Method(),
		Path:       c.Path(),
	}

	result, err := h.pool.Execute(c.Context(), req)

	requestID := uuid.New().String()

	// Build invocation record
	inv := &model.Invocation{
		WorkerID:     worker.ID,
		OwnerID:      worker.OwnerID,
		RequestBytes: requestBytes,
	}

	if err != nil {
		inv.StatusCode = 500
		inv.Error = err.Error()
		inv.ResponseBytes = 0
		go h.recordTelemetry(worker.OwnerID, inv, 0)
		logger.Error("worker execution failed",
			zap.String("worker", worker.Name),
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		return errResponse(c, fiber.StatusInternalServerError, "worker execution failed: "+err.Error())
	}

	responseBytes := int64(len(result.Body))
	inv.StatusCode = result.StatusCode
	inv.Duration = result.Duration
	inv.MemoryUsed = result.MemoryUsed
	inv.ResponseBytes = responseBytes
	if result.Error != "" {
		inv.Error = result.Error
	}

	go h.recordTelemetry(worker.OwnerID, inv, result.Duration.Milliseconds())

	// Telemetry response headers
	for k, v := range result.Headers {
		c.Set(k, v)
	}
	c.Set("X-Cubis-Worker", worker.Name)
	c.Set("X-Cubis-Duration", result.Duration.String())
	c.Set("X-Cubis-Request-ID", requestID)
	c.Set("X-Cubis-Timestamp", time.Now().UTC().Format(time.RFC3339))
	c.Set("X-Cubis-Request-Bytes", itoa64(requestBytes))
	c.Set("X-Cubis-Response-Bytes", itoa64(responseBytes))

	logger.Info("worker invoked",
		zap.String("worker", worker.Name),
		zap.String("request_id", requestID),
		zap.Int("status", result.StatusCode),
		zap.Duration("duration", result.Duration),
		zap.Int64("request_bytes", requestBytes),
		zap.Int64("response_bytes", responseBytes),
	)

	return c.Status(result.StatusCode).Send(result.Body)
}

func (h *InvokeHandler) recordTelemetry(ownerID uuid.UUID, inv *model.Invocation, execTimeMs int64) {
	ctx := context.Background()
	h.workerService.RecordInvocation(ctx, inv)
	h.quotaService.RecordUsage(ctx, ownerID, execTimeMs, inv.RequestBytes, inv.ResponseBytes, inv.Error != "")
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}
