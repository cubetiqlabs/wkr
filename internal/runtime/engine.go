package runtime

import (
	"context"
	"errors"
	"time"
)

var (
	ErrTimeout         = errors.New("worker execution timed out")
	ErrUnsupportedRuntime = errors.New("unsupported runtime")
	ErrExecutionFailed = errors.New("worker execution failed")
)

// ExecutionRequest represents a request to execute a worker.
type ExecutionRequest struct {
	WorkerName string
	Code       string
	Runtime    string
	EntryPoint string
	EnvVars    map[string]string
	Payload    []byte
	Headers    map[string]string
	Method     string
	Path       string
}

// ExecutionResult represents the result of a worker execution.
type ExecutionResult struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	Duration   time.Duration
	MemoryUsed int
	Logs       []string
	Error      string
}

// Engine is the interface for worker execution engines.
type Engine interface {
	Execute(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error)
	Shutdown(ctx context.Context) error
}
