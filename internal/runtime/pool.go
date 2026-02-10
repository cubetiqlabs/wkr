package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cubetiqlabs/wkr/internal/config"
	"github.com/cubetiqlabs/wkr/internal/logger"
	"go.uber.org/zap"
)

// Pool manages concurrent worker executions with resource limits.
type Pool struct {
	engine    Engine
	semaphore chan struct{}
	cfg       config.RuntimeConfig
	mu        sync.RWMutex
	active    int
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

	p.mu.Lock()
	p.active++
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
	}()

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
	logger.Info("worker executed",
		zap.String("worker", req.WorkerName),
		zap.String("runtime", req.Runtime),
		zap.Int("status", result.StatusCode),
		zap.Duration("duration", elapsed),
	)
	return result, nil
}

func (p *Pool) ActiveCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.active
}

func (p *Pool) Shutdown(ctx context.Context) error {
	return p.engine.Shutdown(ctx)
}
