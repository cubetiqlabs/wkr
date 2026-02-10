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
	payload := buildWorkerPayload(req)

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
	payload := buildWorkerPayload(req)

	var stdout, stderr bytes.Buffer

	rt := "deno"
	args := []string{"eval", "--no-remote", wrapJSCode(req.Code, req.EntryPoint)}

	if _, err := exec.LookPath("deno"); err != nil {
		rt = "node"
		args = []string{"-e", wrapJSCode(req.Code, req.EntryPoint)}
	}

	cmd := exec.CommandContext(ctx, rt, args...)
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
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// --- Cubis Context Helpers ---

// Env returns the value of an environment variable.
func Env(key string) string { return os.Getenv(key) }

// EnvOr returns the value of an environment variable or a default.
func EnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" { return v }
	return fallback
}

// Fetch performs an HTTP request and returns the response body as a string.
func Fetch(rawURL string, opts ...map[string]string) (string, error) {
	method := "GET"
	body := ""
	var headers map[string]string
	if len(opts) > 0 {
		if m, ok := opts[0]["method"]; ok { method = strings.ToUpper(m) }
		if b, ok := opts[0]["body"]; ok { body = b }
		headers = opts[0]
	}
	var reqBody io.Reader
	if body != "" { reqBody = strings.NewReader(body) }
	req, err := http.NewRequest(method, rawURL, reqBody)
	if err != nil { return "", err }
	for k, v := range headers {
		if k != "method" && k != "body" { req.Header.Set(k, v) }
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil { return "", err }
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil { return "", err }
	return string(data), nil
}

// Base64Encode encodes a string to base64.
func Base64Encode(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// Base64Decode decodes a base64 string.
func Base64Decode(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	return string(b), err
}

// SHA256Hash returns the hex-encoded SHA-256 hash of a string.
func SHA256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// MD5Hash returns the hex-encoded MD5 hash of a string.
func MD5Hash(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// URLEncode encodes a string for use in a URL query.
func URLEncode(s string) string { return url.QueryEscape(s) }

// URLDecode decodes a URL-encoded string.
func URLDecode(s string) (string, error) { return url.QueryUnescape(s) }

// Log prints a message to stderr (captured as worker logs).
func Log(args ...interface{}) { fmt.Fprintln(os.Stderr, args...) }

// --- User Code ---

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
// --- Cubis Runtime Context ---
const __cubis = {
  _getEnv(key) {
    try { return Deno.env.get(key) || ""; } catch(_) {}
    try { return process.env[key] || ""; } catch(_) {}
    return "";
  },
  _write(s) {
    try { process.stdout.write(s); return; } catch(_) {}
    try { Deno.stdout.writeSync(new TextEncoder().encode(s)); } catch(_) {}
  }
};

// env(key) / env(key, default)
function env(key, fallback) {
  const v = __cubis._getEnv(key);
  return v || (fallback !== undefined ? fallback : "");
}

// fetch is globally available in Deno; polyfill for Node
if (typeof globalThis.fetch === "undefined") {
  globalThis.fetch = async (url, opts) => {
    const http = require(url.startsWith("https") ? "https" : "http");
    return new Promise((resolve, reject) => {
      const req = http.request(url, {method: (opts||{}).method||"GET"}, (res) => {
        let data = "";
        res.on("data", c => data += c);
        res.on("end", () => resolve({
          ok: res.statusCode >= 200 && res.statusCode < 300,
          status: res.statusCode,
          text: async () => data,
          json: async () => JSON.parse(data),
          headers: res.headers
        }));
      });
      req.on("error", reject);
      if ((opts||{}).body) req.write(opts.body);
      req.end();
    });
  };
}

// log(...args) - captured in stderr
function log(...args) { console.error(...args); }

// base64 encode/decode
function btoa(s) {
  try { return globalThis.btoa(s); } catch(_) { return Buffer.from(s).toString("base64"); }
}
function atob(s) {
  try { return globalThis.atob(s); } catch(_) { return Buffer.from(s, "base64").toString(); }
}

// crypto helpers
const crypto = globalThis.crypto || {};
async function sha256(msg) {
  if (crypto.subtle) {
    const buf = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(msg));
    return [...new Uint8Array(buf)].map(b => b.toString(16).padStart(2,"0")).join("");
  }
  const c = require("crypto");
  return c.createHash("sha256").update(msg).digest("hex");
}
async function md5(msg) {
  try { const c = require("crypto"); return c.createHash("md5").update(msg).digest("hex"); }
  catch(_) { throw new Error("md5 not available in this runtime"); }
}

// uuid v4
function uuid() {
  if (crypto.randomUUID) return crypto.randomUUID();
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, c => {
    const r = Math.random()*16|0;
    return (c==="x"?r:(r&0x3|0x8)).toString(16);
  });
}

// sleep(ms)
function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }

// jsonParse / jsonStringify with error handling
function jsonParse(s, fallback) { try { return JSON.parse(s); } catch(_) { return fallback !== undefined ? fallback : null; } }
function jsonStringify(v, pretty) { return pretty ? JSON.stringify(v, null, 2) : JSON.stringify(v); }

// urlEncode / urlDecode
function urlEncode(s) { return encodeURIComponent(s); }
function urlDecode(s) { return decodeURIComponent(s); }

// --- User Code ---

%s

// --- Execute ---
(async () => {
  const payload = jsonParse(__cubis._getEnv("CUBIS_PAYLOAD"), {});
  let result;
  const fn = %s;
  result = fn(payload);
  if (result instanceof Promise) result = await result;

  const output = jsonStringify({
    status: 200,
    headers: {"Content-Type": "application/json"},
    body: typeof result === "string" ? result : jsonStringify(result)
  });
  __cubis._write(output);
})();
`, code, entryPoint)
}

func parseWorkerOutput(data []byte, duration time.Duration) (*ExecutionResult, error) {
	var output struct {
		Status  int               `json:"status"`
		Headers map[string]string `json:"headers"`
		Body    interface{}       `json:"body"`
	}

	if err := json.Unmarshal(data, &output); err != nil {
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
