package runtime

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/cubetiqlabs/wkr/internal/config"
	"github.com/cubetiqlabs/wkr/internal/logger"
	"go.uber.org/zap"
)

type Pool struct {
	engine    Engine
	semaphore chan struct{}
	cfg       config.RuntimeConfig
	active    atomic.Int64
}

func NewPool(engine Engine, cfg config.RuntimeConfig) *Pool {
	return &Pool{
		engine:    engine,
		semaphore: make(chan struct{}, cfg.MaxConcurrentWorkers),
		cfg:       cfg,
	}
}

func (p *Pool) Execute(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	select {
	case p.semaphore <- struct{}{}:
		defer func() { <-p.semaphore }()
	case <-ctx.Done():
		return nil, fmt.Errorf("pool: %w", ctx.Err())
	}

	p.active.Add(1)
	defer p.active.Add(-1)

	timeout := p.cfg.MaxExecutionTime
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	result, err := p.engine.Execute(execCtx, req)
	elapsed := time.Since(start)

	if err != nil {
		logger.Error("worker execution failed",
			zap.String("worker", req.WorkerName),
			zap.String("runtime", req.Runtime),
			zap.Duration("duration", elapsed),
			zap.Error(err),
		)
		return nil, err
	}

	result.Duration = elapsed
	if elapsed > 500*time.Millisecond {
		logger.Warn("slow worker execution",
			zap.String("worker", req.WorkerName),
			zap.Duration("duration", elapsed),
		)
	}
	return result, nil
}

func (p *Pool) ActiveCount() int {
	return int(p.active.Load())
}

func (p *Pool) Shutdown(ctx context.Context) error {
	return p.engine.Shutdown(ctx)
}
