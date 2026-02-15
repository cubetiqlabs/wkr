package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/cubetiqlabs/wkr/internal/edge"
	"github.com/cubetiqlabs/wkr/internal/logger"
	"github.com/cubetiqlabs/wkr/internal/metrics"
	"github.com/cubetiqlabs/wkr/internal/model"
	"github.com/cubetiqlabs/wkr/internal/runtime"
	"github.com/cubetiqlabs/wkr/internal/security"
	"github.com/cubetiqlabs/wkr/internal/service"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type InvokeHandler struct {
	workerService *service.WorkerService
	quotaService  *service.QuotaService
	pool          *runtime.Pool
	auditor       *security.Auditor
	edgeRouter    *edge.Router
	nodeID        string
}

func NewInvokeHandler(
	workerService *service.WorkerService,
	quotaService *service.QuotaService,
	pool *runtime.Pool,
	auditor *security.Auditor,
	edgeRouter *edge.Router,
	nodeID string,
) *InvokeHandler {
	return &InvokeHandler{
		workerService: workerService,
		quotaService:  quotaService,
		pool:          pool,
		auditor:       auditor,
		edgeRouter:    edgeRouter,
		nodeID:        nodeID,
	}
}

// WorkerErrorDetail is the structured error response for worker execution failures.
type WorkerErrorDetail struct {
	Success   bool     `json:"success"`
	Error     string   `json:"error"`
	RequestID string   `json:"request_id"`
	Worker    string   `json:"worker"`
	Runtime   string   `json:"runtime"`
	NodeID    string   `json:"node_id,omitempty"`
	Logs      []string `json:"logs,omitempty"`
}

func (h *InvokeHandler) Invoke(c fiber.Ctx) error {
	name := c.Params("name")
	if name == "" {
		return errResponse(c, fiber.StatusBadRequest, "worker name required")
	}

	// Scoped invocation: /invoke/@username/worker-name
	username := c.Params("username")
	var worker *model.Worker
	var err error
	if username != "" {
		worker, err = h.workerService.GetByUsernameAndName(c.Context(), username, name)
	} else {
		worker, err = h.workerService.GetByName(c.Context(), name)
	}
	if err != nil {
		return errResponse(c, fiber.StatusNotFound, "worker not found")
	}

	// Enforce account quota — fail closed on unexpected errors
	if err := h.quotaService.CheckInvocationAllowed(c.Context(), worker.OwnerID); err != nil {
		switch err {
		case service.ErrQuotaRequestsExceeded:
			metrics.QuotaRejectedTotal.WithLabelValues("requests", h.nodeID).Inc()
			return errResponse(c, fiber.StatusTooManyRequests, "daily request limit exceeded")
		case service.ErrQuotaExecTimeExceeded:
			metrics.QuotaRejectedTotal.WithLabelValues("exec_time", h.nodeID).Inc()
			return errResponse(c, fiber.StatusTooManyRequests, "daily execution time limit exceeded")
		case service.ErrQuotaBandwidthExceeded:
			metrics.QuotaRejectedTotal.WithLabelValues("bandwidth", h.nodeID).Inc()
			return errResponse(c, fiber.StatusTooManyRequests, "daily bandwidth limit exceeded")
		default:
			logger.Error("quota check failed", zap.Error(err))
			return errResponse(c, fiber.StatusServiceUnavailable, "service temporarily unavailable")
		}
	}

	headers := make(map[string]string)
	c.Request().Header.VisitAll(func(key, value []byte) {
		headers[string(key)] = string(value)
	})

	reqBody := c.Body()

	// Edge routing: forward to a less-loaded node if available
	isForwarded := c.Get("X-Cubis-Edge-Route") == "true"
	if h.edgeRouter != nil {
		if target := h.edgeRouter.ShouldForwardToEdge(isForwarded); target != nil {
			logger.Info("forwarding to edge node",
				zap.String("worker", name),
				zap.String("target_node", target.NodeID),
				zap.String("target_endpoint", target.Endpoint),
			)
			status, body, respHeaders, err := h.edgeRouter.ForwardRequest(c.Context(), target, name, c.Method(), reqBody, headers, string(c.Request().URI().QueryString()))
			if err != nil {
				logger.Error("edge forward failed, executing locally",
					zap.String("target_node", target.NodeID),
					zap.String("target_endpoint", target.Endpoint),
					zap.Error(err),
				)
			} else {
				for k, v := range respHeaders {
					c.Set(k, v)
				}
				return c.Status(status).Send(body)
			}
		} else if !isForwarded {
			logger.Debug("no edge target, executing locally",
				zap.String("worker", name),
				zap.String("node_id", h.nodeID),
			)
		}
	}

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
		Query:      string(c.Request().URI().QueryString()),
	}

	result, err := h.pool.Execute(c.Context(), req)
	requestID := uuid.New().String()

	inv := &model.Invocation{
		WorkerID:     worker.ID,
		OwnerID:      worker.OwnerID,
		RequestBytes: requestBytes,
	}

	setTelemetry := func(respBytes int64, dur time.Duration) {
		c.Set("X-Cubis-Worker", worker.Name)
		c.Set("X-Cubis-Request-ID", requestID)
		c.Set("X-Cubis-Node-ID", h.nodeID)
		c.Set("X-Cubis-Duration", dur.String())
		c.Set("X-Cubis-Timestamp", time.Now().UTC().Format(time.RFC3339))
		c.Set("X-Cubis-Request-Bytes", strconv.FormatInt(requestBytes, 10))
		c.Set("X-Cubis-Response-Bytes", strconv.FormatInt(respBytes, 10))
	}

	// Pool-level error
	if err != nil {
		inv.StatusCode = 500
		inv.Error = err.Error()
		go h.recordTelemetry(worker.OwnerID, inv, 0, requestBytes, 0)
		setTelemetry(0, 0)
		logger.Error("worker execution failed",
			zap.String("worker", worker.Name),
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(WorkerErrorDetail{
			Error:     err.Error(),
			RequestID: requestID,
			Worker:    worker.Name,
			Runtime:   string(worker.Runtime),
			NodeID:    h.nodeID,
		})
	}

	responseBytes := int64(len(result.Body))
	inv.StatusCode = result.StatusCode
	inv.Duration = result.Duration
	inv.MemoryUsed = result.MemoryUsed
	inv.ResponseBytes = responseBytes
	inv.Error = result.Error

	go h.recordTelemetry(worker.OwnerID, inv, result.Duration.Milliseconds(), requestBytes, responseBytes)
	setTelemetry(responseBytes, result.Duration)

	// Security blocked
	if result.StatusCode == 403 && result.Error != "" {
		h.auditor.Log(c.Context(), "code_blocked", "warn", result.Error, c.IP(), worker.OwnerID, worker.ID)
		return c.Status(fiber.StatusForbidden).JSON(WorkerErrorDetail{
			Error:     result.Error,
			RequestID: requestID,
			Worker:    worker.Name,
			Runtime:   string(worker.Runtime),
			NodeID:    h.nodeID,
		})
	}

	// Worker runtime error
	if result.Error != "" {
		logger.Warn("worker runtime error",
			zap.String("worker", worker.Name),
			zap.String("request_id", requestID),
			zap.String("error", result.Error),
			zap.Strings("logs", result.Logs),
			zap.Duration("duration", result.Duration),
		)
		return c.Status(result.StatusCode).JSON(WorkerErrorDetail{
			Error:     result.Error,
			RequestID: requestID,
			Worker:    worker.Name,
			Runtime:   string(worker.Runtime),
			NodeID:    h.nodeID,
			Logs:      result.Logs,
		})
	}

	for k, v := range result.Headers {
		c.Set(k, v)
	}

	logger.Info("worker invoked",
		zap.String("worker", worker.Name),
		zap.String("request_id", requestID),
		zap.String("node_id", h.nodeID),
		zap.Int("status", result.StatusCode),
		zap.Duration("duration", result.Duration),
	)

	return c.Status(result.StatusCode).Send(result.Body)
}

func (h *InvokeHandler) recordTelemetry(ownerID uuid.UUID, inv *model.Invocation, execTimeMs, reqBytes, respBytes int64) {
	ctx := context.Background()
	h.workerService.RecordInvocation(ctx, inv)
	h.quotaService.RecordUsage(ctx, ownerID, execTimeMs, inv.RequestBytes, inv.ResponseBytes, inv.Error != "")
	metrics.RequestBytesTotal.WithLabelValues(h.nodeID).Add(float64(reqBytes))
	metrics.ResponseBytesTotal.WithLabelValues(h.nodeID).Add(float64(respBytes))
}
