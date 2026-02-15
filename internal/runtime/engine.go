package runtime

import (
	"context"
	"errors"
	"time"
)

var (
	ErrTimeout            = errors.New("worker execution timed out")
	ErrUnsupportedRuntime = errors.New("unsupported runtime")
	ErrExecutionFailed    = errors.New("worker execution failed")
)

type ExecutionRequest struct {
	WorkerName string
	Code       string
	CodeHash   string // used as cache key for compiled binaries
	Runtime    string
	EntryPoint string
	EnvVars    map[string]string
	Payload    []byte
	Headers    map[string]string
	Method     string
	Path       string
	Query      string
}

type ExecutionResult struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	Duration   time.Duration
	MemoryUsed int
	Logs       []string // stderr lines (user log() calls + runtime errors)
	Error      string   // structured error message (not just exit code)
}

type Engine interface {
	Execute(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error)
	Shutdown(ctx context.Context) error
}
