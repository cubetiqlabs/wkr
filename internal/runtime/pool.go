package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cubetiqlabs/wkr/internal/config"
	"github.com/cubetiqlabs/wkr/internal/logger"
	"github.com/cubetiqlabs/wkr/internal/metrics"
	"github.com/cubetiqlabs/wkr/internal/security"
	"go.uber.org/zap"
)

type Pool struct {
	engine    Engine
	semaphore chan struct{}
	cfg       config.RuntimeConfig
	active    atomic.Int64
	validator *security.Validator
	nodeID    string
	region    string
}

func NewPool(engine Engine, cfg config.RuntimeConfig, validator *security.Validator, nodeID, region string) *Pool {
	metrics.PoolCapacity.WithLabelValues(nodeID).Set(float64(cfg.MaxConcurrentWorkers))
	return &Pool{
		engine:    engine,
		semaphore: make(chan struct{}, cfg.MaxConcurrentWorkers),
		cfg:       cfg,
		validator: validator,
		nodeID:    nodeID,
		region:    region,
	}
}

func (p *Pool) Execute(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	// Security validation before acquiring pool slot
	if p.validator != nil {
		if err := p.validator.ValidateCode(req.Code, req.Runtime, req.CodeHash); err != nil {
			metrics.SecurityBlocked.WithLabelValues("code_validation", req.WorkerName, p.nodeID).Inc()
			metrics.InvocationsFailedTotal.WithLabelValues(req.WorkerName, req.Runtime, "security_blocked", p.region, p.nodeID).Inc()
			metrics.InvocationsTotal.WithLabelValues(req.WorkerName, req.Runtime, "blocked", p.region, p.nodeID).Inc()
			return &ExecutionResult{
				StatusCode: 403,
				Error:      "security: " + err.Error(),
			}, nil
		}
	}

	// Wait for pool slot
	waitStart := time.Now()
	select {
	case p.semaphore <- struct{}{}:
		defer func() { <-p.semaphore }()
	case <-ctx.Done():
		metrics.PoolRejected.WithLabelValues(p.nodeID).Inc()
		metrics.InvocationsFailedTotal.WithLabelValues(req.WorkerName, req.Runtime, "pool_full", p.region, p.nodeID).Inc()
		metrics.InvocationsTotal.WithLabelValues(req.WorkerName, req.Runtime, "rejected", p.region, p.nodeID).Inc()
		return nil, fmt.Errorf("pool: %w", ctx.Err())
	}
	metrics.PoolQueueWait.WithLabelValues(p.nodeID).Observe(time.Since(waitStart).Seconds())

	p.active.Add(1)
	defer func() {
		p.active.Add(-1)
		metrics.ActiveWorkers.WithLabelValues(p.nodeID, p.region).Set(float64(p.active.Load()))
	}()
	metrics.ActiveWorkers.WithLabelValues(p.nodeID, p.region).Set(float64(p.active.Load()))

	timeout := p.cfg.MaxExecutionTime
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	result, err := p.engine.Execute(execCtx, req)
	elapsed := time.Since(start)

	// Engine-level error (should not happen normally)
	if err != nil {
		errType := classifyError(err)
		metrics.InvocationsFailedTotal.WithLabelValues(req.WorkerName, req.Runtime, errType, p.region, p.nodeID).Inc()
		metrics.InvocationsTotal.WithLabelValues(req.WorkerName, req.Runtime, "error", p.region, p.nodeID).Inc()
		if errType == "timeout" {
			metrics.InvocationTimeouts.WithLabelValues(req.WorkerName, req.Runtime, p.region).Inc()
		}
		logger.Error("worker execution failed",
			zap.String("worker", req.WorkerName),
			zap.String("runtime", req.Runtime),
			zap.Duration("duration", elapsed),
			zap.Error(err),
		)
		return nil, err
	}

	result.Duration = elapsed
	metrics.InvocationDuration.WithLabelValues(req.WorkerName, req.Runtime, p.region).Observe(elapsed.Seconds())

	if result.Error != "" {
		errType := "runtime_error"
		if strings.Contains(result.Error, "compile failed") {
			errType = "compile_error"
			metrics.InvocationCompileErrors.WithLabelValues(req.WorkerName, p.nodeID).Inc()
		} else {
			metrics.InvocationRuntimeErrors.WithLabelValues(req.WorkerName, req.Runtime, p.region).Inc()
		}
		metrics.InvocationsFailedTotal.WithLabelValues(req.WorkerName, req.Runtime, errType, p.region, p.nodeID).Inc()
		metrics.InvocationsTotal.WithLabelValues(req.WorkerName, req.Runtime, "error", p.region, p.nodeID).Inc()
	} else {
		metrics.InvocationsSuccessTotal.WithLabelValues(req.WorkerName, req.Runtime, p.region, p.nodeID).Inc()
		metrics.InvocationsTotal.WithLabelValues(req.WorkerName, req.Runtime, "success", p.region, p.nodeID).Inc()
	}

	if elapsed > 500*time.Millisecond {
		logger.Warn("slow worker execution",
			zap.String("worker", req.WorkerName),
			zap.Duration("duration", elapsed),
		)
	}
	return result, nil
}

func classifyError(err error) string {
	s := err.Error()
	if strings.Contains(s, "timed out") || strings.Contains(s, "deadline exceeded") {
		return "timeout"
	}
	if strings.Contains(s, "context canceled") {
		return "cancelled"
	}
	return "engine_error"
}

func (p *Pool) ActiveCount() int {
	return int(p.active.Load())
}

func (p *Pool) Shutdown(ctx context.Context) error {
	return p.engine.Shutdown(ctx)
}
