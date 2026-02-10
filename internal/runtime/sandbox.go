package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// SandboxEngine executes workers in isolated subprocess sandboxes.
// Optimizations:
//   - Go binaries are compiled once and cached by code hash
//   - JS runtime (deno/node) is detected once at startup
//   - Host environment vars are cached once
//   - Buffer pools reduce GC pressure under load
type SandboxEngine struct {
	// Go binary cache: codeHash -> compiled binary path
	goBinCache sync.Map
	goCacheDir string

	// JS runtime detection (resolved once)
	jsRuntime string // "deno" or "node"

	// Cached host env (immutable after init)
	hostEnv []string

	// Buffer pool for stdout/stderr
	bufPool sync.Pool
}

func NewSandboxEngine() *SandboxEngine {
	cacheDir := filepath.Join(os.TempDir(), "cubis-cache")
	os.MkdirAll(cacheDir, 0o755)

	// Detect JS runtime once
	jsRT := "node"
	if _, err := exec.LookPath("deno"); err == nil {
		jsRT = "deno"
	}

	// Cache host env once
	hostEnv := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"GOPATH=" + os.Getenv("GOPATH"),
		"GOROOT=" + os.Getenv("GOROOT"),
		"GOMODCACHE=" + os.Getenv("GOMODCACHE"),
		"GOCACHE=" + os.Getenv("GOCACHE"),
	}

	return &SandboxEngine{
		goCacheDir: cacheDir,
		jsRuntime:  jsRT,
		hostEnv:    hostEnv,
		bufPool: sync.Pool{
			New: func() interface{} { return new(bytes.Buffer) },
		},
	}
}

func (e *SandboxEngine) getBuf() *bytes.Buffer {
	b := e.bufPool.Get().(*bytes.Buffer)
	b.Reset()
	return b
}

func (e *SandboxEngine) putBuf(b *bytes.Buffer) {
	if b.Cap() < 1<<20 { // don't pool buffers > 1MB
		e.bufPool.Put(b)
	}
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

// ---------- Go Runtime (compiled binary cache) ----------

func (e *SandboxEngine) executeGo(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	binPath, err := e.getOrCompileGo(ctx, req)
	if err != nil {
		return &ExecutionResult{
			StatusCode: 500,
			Body:       []byte(err.Error()),
			Error:      err.Error(),
		}, nil
	}

	payload := buildWorkerPayload(req)
	env := e.buildWorkerEnv(req.EnvVars)
	env = append(env, "CUBIS_PAYLOAD="+string(payload))

	stdout := e.getBuf()
	stderr := e.getBuf()
	defer e.putBuf(stdout)
	defer e.putBuf(stderr)

	cmd := exec.CommandContext(ctx, binPath)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = env

	start := time.Now()
	err = cmd.Run()
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
			Body:       copyBytes(stderr.Bytes()),
			Duration:   duration,
			Error:      err.Error(),
			Logs:       []string{stderr.String()},
		}, nil
	}

	return parseWorkerOutput(copyBytes(stdout.Bytes()), duration)
}

// getOrCompileGo returns a cached binary path or compiles one.
func (e *SandboxEngine) getOrCompileGo(ctx context.Context, req *ExecutionRequest) (string, error) {
	cacheKey := req.CodeHash
	if cacheKey == "" {
		h := sha256.Sum256([]byte(req.Code + req.EntryPoint))
		cacheKey = fmt.Sprintf("%x", h)
	}

	// Fast path: cached binary exists
	if cached, ok := e.goBinCache.Load(cacheKey); ok {
		binPath := cached.(string)
		if _, err := os.Stat(binPath); err == nil {
			return binPath, nil
		}
		e.goBinCache.Delete(cacheKey) // stale entry
	}

	// Slow path: compile
	srcPath := filepath.Join(e.goCacheDir, cacheKey+".go")
	binPath := filepath.Join(e.goCacheDir, cacheKey)

	if err := os.WriteFile(srcPath, []byte(wrapGoCode(req.Code, req.EntryPoint)), 0o644); err != nil {
		return "", fmt.Errorf("write source: %w", err)
	}
	defer os.Remove(srcPath)

	stderr := e.getBuf()
	defer e.putBuf(stderr)

	cmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, srcPath)
	cmd.Stderr = stderr
	cmd.Env = e.hostEnv

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("compile failed: %s", stderr.String())
	}

	e.goBinCache.Store(cacheKey, binPath)
	return binPath, nil
}

// ---------- JS/TS Runtime ----------

func (e *SandboxEngine) executeJS(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	payload := buildWorkerPayload(req)
	code := wrapJSCode(req.Code, req.EntryPoint)

	var args []string
	if e.jsRuntime == "deno" {
		args = []string{"eval", "--no-remote", code}
	} else {
		args = []string{"-e", code}
	}

	env := e.buildWorkerEnv(req.EnvVars)
	env = append(env, "CUBIS_PAYLOAD="+string(payload))

	stdout := e.getBuf()
	stderr := e.getBuf()
	defer e.putBuf(stdout)
	defer e.putBuf(stderr)

	cmd := exec.CommandContext(ctx, e.jsRuntime, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = env

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
			Body:       copyBytes(stderr.Bytes()),
			Duration:   duration,
			Error:      err.Error(),
			Logs:       []string{stderr.String()},
		}, nil
	}

	return parseWorkerOutput(copyBytes(stdout.Bytes()), duration)
}

// ---------- Shared helpers ----------

func (e *SandboxEngine) buildWorkerEnv(vars map[string]string) []string {
	env := make([]string, len(e.hostEnv), len(e.hostEnv)+len(vars)+1)
	copy(env, e.hostEnv)
	for k, v := range vars {
		env = append(env, k+"="+v)
	}
	return env
}

func (e *SandboxEngine) Shutdown(_ context.Context) error {
	// Clean up compiled Go binaries
	os.RemoveAll(e.goCacheDir)
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

// copyBytes returns a copy that's safe to use after buffer is recycled.
func copyBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
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

func Env(key string) string { return os.Getenv(key) }
func EnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" { return v }
	return fallback
}
func Fetch(rawURL string, opts ...map[string]string) (string, error) {
	method := "GET"; body := ""
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
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil { return "", err }
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return string(data), err
}
func Base64Encode(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
func Base64Decode(s string) (string, error) { b, err := base64.StdEncoding.DecodeString(s); return string(b), err }
func SHA256Hash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func MD5Hash(s string) string { h := md5.Sum([]byte(s)); return hex.EncodeToString(h[:]) }
func URLEncode(s string) string { return url.QueryEscape(s) }
func URLDecode(s string) (string, error) { return url.QueryUnescape(s) }
func Log(args ...interface{}) { fmt.Fprintln(os.Stderr, args...) }

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
	return fmt.Sprintf(`const __cubis={_getEnv(k){try{return Deno.env.get(k)||""}catch(_){}try{return process.env[k]||""}catch(_){}return""},_write(s){try{process.stdout.write(s);return}catch(_){}try{Deno.stdout.writeSync(new TextEncoder().encode(s))}catch(_){}}};
function env(k,d){const v=__cubis._getEnv(k);return v||(d!==undefined?d:"")}
function log(...a){console.error(...a)}
function btoa(s){try{return globalThis.btoa(s)}catch(_){return Buffer.from(s).toString("base64")}}
function atob(s){try{return globalThis.atob(s)}catch(_){return Buffer.from(s,"base64").toString()}}
const crypto=globalThis.crypto||{};
async function sha256(m){if(crypto.subtle){const b=await crypto.subtle.digest("SHA-256",new TextEncoder().encode(m));return[...new Uint8Array(b)].map(b=>b.toString(16).padStart(2,"0")).join("")}const c=require("crypto");return c.createHash("sha256").update(m).digest("hex")}
async function md5(m){const c=require("crypto");return c.createHash("md5").update(m).digest("hex")}
function uuid(){if(crypto.randomUUID)return crypto.randomUUID();return"xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g,c=>{const r=Math.random()*16|0;return(c==="x"?r:(r&3|8)).toString(16)})}
function sleep(ms){return new Promise(r=>setTimeout(r,ms))}
function jsonParse(s,f){try{return JSON.parse(s)}catch(_){return f!==undefined?f:null}}
function jsonStringify(v,p){return p?JSON.stringify(v,null,2):JSON.stringify(v)}
function urlEncode(s){return encodeURIComponent(s)}
function urlDecode(s){return decodeURIComponent(s)}
if(typeof globalThis.fetch==="undefined"){globalThis.fetch=async(u,o)=>{const h=require(u.startsWith("https")?"https":"http");return new Promise((res,rej)=>{const r=h.request(u,{method:(o||{}).method||"GET"},s=>{let d="";s.on("data",c=>d+=c);s.on("end",()=>res({ok:s.statusCode>=200&&s.statusCode<300,status:s.statusCode,text:async()=>d,json:async()=>JSON.parse(d),headers:s.headers}))});r.on("error",rej);if((o||{}).body)r.write(o.body);r.end()})}}

%s

;(async()=>{const payload=jsonParse(__cubis._getEnv("CUBIS_PAYLOAD"),{});let result=%s(payload);if(result instanceof Promise)result=await result;const output=jsonStringify({status:200,headers:{"Content-Type":"application/json"},body:typeof result==="string"?result:jsonStringify(result)});__cubis._write(output)})();
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
