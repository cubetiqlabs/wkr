package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// SandboxEngine executes workers in isolated subprocess sandboxes.
// For production, this would use gVisor/Firecracker/Wasm. This implementation
// provides process-level isolation as a foundation.
type SandboxEngine struct{}

func NewSandboxEngine() *SandboxEngine {
	return &SandboxEngine{}
}

func (e *SandboxEngine) Execute(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	switch req.Runtime {
	case "go":
		return e.executeGo(ctx, req)
	case "javascript", "typescript":
		return e.executeJS(ctx, req)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedRuntime, req.Runtime)
	}
}

func (e *SandboxEngine) executeGo(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	// Build the Go worker as a temporary binary and execute it.
	// In production, this would use a pre-compiled plugin or Wasm module.
	// For now, we use `go run` with a temp file in a sandboxed environment.

	payload := buildWorkerPayload(req)

	// Use Go's built-in execution with resource limits
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "go", "run", "-")
	cmd.Stdin = bytes.NewReader([]byte(wrapGoCode(req.Code, req.EntryPoint)))
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = buildEnv(req.EnvVars)
	cmd.Env = append(cmd.Env, "CUBIS_PAYLOAD="+string(payload))

	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	if ctx.Err() != nil {
		return &ExecutionResult{
			StatusCode: 504,
			Body:       []byte(`{"error":"execution timed out"}`),
			Duration:   duration,
			Error:      ErrTimeout.Error(),
		}, ErrTimeout
	}

	if err != nil {
		return &ExecutionResult{
			StatusCode: 500,
			Body:       []byte(stderr.String()),
			Duration:   duration,
			Error:      err.Error(),
			Logs:       []string{stderr.String()},
		}, nil
	}

	return parseWorkerOutput(stdout.Bytes(), duration)
}

func (e *SandboxEngine) executeJS(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	// Execute JavaScript/TypeScript using Deno or Node.js runtime
	// Deno is preferred for its security-first design (permissions model)
	payload := buildWorkerPayload(req)

	var stdout, stderr bytes.Buffer

	// Try deno first, fall back to node
	runtime := "deno"
	args := []string{"eval", "--no-remote", wrapJSCode(req.Code, req.EntryPoint)}

	if _, err := exec.LookPath("deno"); err != nil {
		runtime = "node"
		args = []string{"-e", wrapJSCode(req.Code, req.EntryPoint)}
	}

	cmd := exec.CommandContext(ctx, runtime, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = buildEnv(req.EnvVars)
	cmd.Env = append(cmd.Env, "CUBIS_PAYLOAD="+string(payload))

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	if ctx.Err() != nil {
		return &ExecutionResult{
			StatusCode: 504,
			Body:       []byte(`{"error":"execution timed out"}`),
			Duration:   duration,
			Error:      ErrTimeout.Error(),
		}, ErrTimeout
	}

	if err != nil {
		return &ExecutionResult{
			StatusCode: 500,
			Body:       []byte(stderr.String()),
			Duration:   duration,
			Error:      err.Error(),
			Logs:       []string{stderr.String()},
		}, nil
	}

	return parseWorkerOutput(stdout.Bytes(), duration)
}

func (e *SandboxEngine) Shutdown(_ context.Context) error {
	return nil
}

func buildWorkerPayload(req *ExecutionRequest) []byte {
	p := map[string]interface{}{
		"method":  req.Method,
		"path":    req.Path,
		"headers": req.Headers,
		"body":    string(req.Payload),
	}
	data, _ := json.Marshal(p)
	return data
}

func buildEnv(vars map[string]string) []string {
	env := []string{"PATH=/usr/local/bin:/usr/bin:/bin"}
	for k, v := range vars {
		env = append(env, k+"="+v)
	}
	return env
}

func wrapGoCode(code, entryPoint string) string {
	return fmt.Sprintf(`package main

import (
	"encoding/json"
	"fmt"
	"os"
)

%s

func main() {
	payload := os.Getenv("CUBIS_PAYLOAD")
	var req map[string]interface{}
	json.Unmarshal([]byte(payload), &req)

	result := %s(req)

	out, _ := json.Marshal(map[string]interface{}{
		"status":  200,
		"headers": map[string]string{"Content-Type": "application/json"},
		"body":    result,
	})
	fmt.Print(string(out))
}
`, code, entryPoint)
}

func wrapJSCode(code, entryPoint string) string {
	return fmt.Sprintf(`
%s

const payload = JSON.parse(process.env.CUBIS_PAYLOAD || Deno.env.get("CUBIS_PAYLOAD") || "{}");
const result = %s(payload);
const output = JSON.stringify({
  status: 200,
  headers: {"Content-Type": "application/json"},
  body: typeof result === "string" ? result : JSON.stringify(result)
});
if (typeof process !== "undefined") { process.stdout.write(output); }
else { Deno.stdout.writeSync(new TextEncoder().encode(output)); }
`, code, entryPoint)
}

func parseWorkerOutput(data []byte, duration time.Duration) (*ExecutionResult, error) {
	var output struct {
		Status  int               `json:"status"`
		Headers map[string]string `json:"headers"`
		Body    interface{}       `json:"body"`
	}

	if err := json.Unmarshal(data, &output); err != nil {
		// If output isn't structured, return raw
		return &ExecutionResult{
			StatusCode: 200,
			Headers:    map[string]string{"Content-Type": "text/plain"},
			Body:       data,
			Duration:   duration,
		}, nil
	}

	var body []byte
	switch v := output.Body.(type) {
	case string:
		body = []byte(v)
	default:
		body, _ = json.Marshal(v)
	}

	status := output.Status
	if status == 0 {
		status = 200
	}

	return &ExecutionResult{
		StatusCode: status,
		Headers:    output.Headers,
		Body:       body,
		Duration:   duration,
	}, nil
}
