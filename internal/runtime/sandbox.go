package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cubetiqlabs/wkr/internal/metrics"
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

	// Node identity for metrics
	nodeID string
}

func NewSandboxEngine(nodeID string) *SandboxEngine {
	cacheDir := filepath.Join(os.TempDir(), "cubis-cache")
	os.MkdirAll(cacheDir, 0o700)

	// Detect JS runtime once (default, may be overridden per-request by version)
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
		nodeID:     nodeID,
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
	case "python":
		return e.executePython(ctx, req)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedRuntime, req.Runtime)
	}
}

// ---------- Go Runtime (compiled binary cache) ----------

func (e *SandboxEngine) executeGo(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	goBin := resolveRuntime("go", req.RuntimeVersion)

	binPath, err := e.getOrCompileGo(ctx, req, goBin)
	if err != nil {
		return &ExecutionResult{
			StatusCode: 500,
			Body:       []byte(err.Error()),
			Error:      err.Error(),
		}, nil
	}

	payload := buildWorkerPayload(req)
	env := e.buildWorkerEnv(req.EnvVars)

	stdout := e.getBuf()
	stderr := e.getBuf()
	defer e.putBuf(stdout)
	defer e.putBuf(stderr)

	cmd := exec.CommandContext(ctx, binPath)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = env

	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)
	logs := parseStderrLogs(stderr)

	if ctx.Err() != nil {
		return &ExecutionResult{
			StatusCode: 504,
			Duration:   duration,
			Error:      "execution timed out",
			Logs:       logs,
		}, ErrTimeout
	}

	if err != nil {
		errMsg := extractRuntimeError(stderr.String(), "go")
		return &ExecutionResult{
			StatusCode: 500,
			Duration:   duration,
			Error:      errMsg,
			Logs:       logs,
		}, nil
	}

	result, parseErr := parseWorkerOutput(copyBytes(stdout.Bytes()), duration)
	result.Logs = logs
	return result, parseErr
}

// getOrCompileGo returns a cached binary path or compiles one.
func (e *SandboxEngine) getOrCompileGo(ctx context.Context, req *ExecutionRequest, goBin string) (string, error) {
	cacheKey := req.CodeHash
	if cacheKey == "" {
		h := sha256.Sum256([]byte(req.Code + req.EntryPoint + req.RuntimeVersion + req.Dependencies))
		cacheKey = fmt.Sprintf("%x", h)
	}

	// Fast path: cached binary exists
	if cached, ok := e.goBinCache.Load(cacheKey); ok {
		binPath := cached.(string)
		if _, err := os.Stat(binPath); err == nil {
			metrics.GoCacheHits.WithLabelValues(e.nodeID).Inc()
			return binPath, nil
		}
		e.goBinCache.Delete(cacheKey) // stale entry
	}

	metrics.GoCacheMisses.WithLabelValues(e.nodeID).Inc()

	// Slow path: compile
	buildDir := filepath.Join(e.goCacheDir, "go-"+cacheKey)
	os.MkdirAll(buildDir, 0o755)
	srcPath := filepath.Join(buildDir, "main.go")
	binPath := filepath.Join(e.goCacheDir, cacheKey)

	if err := os.WriteFile(srcPath, []byte(wrapGoCode(req.Code, req.EntryPoint)), 0o644); err != nil {
		return "", fmt.Errorf("write source: %w", err)
	}

	// Install dependencies if provided (write go.mod, go mod download)
	if req.Dependencies != "" {
		depsDir, err := installDeps(ctx, "go", req.Dependencies, req.PackageManager, goBin, e.hostEnv)
		if err != nil {
			return "", fmt.Errorf("install deps: %w", err)
		}
		// Copy go.mod/go.sum into build dir for compilation
		for _, f := range []string{"go.mod", "go.sum"} {
			src := filepath.Join(depsDir, f)
			if data, err := os.ReadFile(src); err == nil {
				os.WriteFile(filepath.Join(buildDir, f), data, 0o644)
			}
		}
	}

	stderr := e.getBuf()
	defer e.putBuf(stderr)

	cmd := exec.CommandContext(ctx, goBin, "build", "-o", binPath, srcPath)
	cmd.Dir = buildDir
	cmd.Stderr = stderr
	cmd.Env = e.hostEnv

	compileStart := time.Now()
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("compile failed: %s", stderr.String())
	}
	metrics.GoCompileDuration.WithLabelValues(e.nodeID).Observe(time.Since(compileStart).Seconds())

	e.goBinCache.Store(cacheKey, binPath)
	return binPath, nil
}

// ---------- JS/TS Runtime ----------

func (e *SandboxEngine) executeJS(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	payload := buildWorkerPayload(req)
	code := wrapJSCode(req.Code, req.EntryPoint)

	jsBin := resolveRuntime(req.Runtime, req.RuntimeVersion)
	isDeno := strings.Contains(jsBin, "deno") || e.jsRuntime == "deno"

	env := e.buildWorkerEnv(req.EnvVars)

	// Install npm dependencies if provided
	hasDeps := req.Dependencies != ""
	var depsDir string
	if hasDeps {
		var err error
		depsDir, err = installDeps(ctx, req.Runtime, req.Dependencies, req.PackageManager, jsBin, env)
		if err != nil {
			return &ExecutionResult{
				StatusCode: 500,
				Error:      "install deps: " + err.Error(),
			}, nil
		}
		env = append(env, "NODE_PATH="+filepath.Join(depsDir, "node_modules"))
	}

	var args []string
	var tmpFile string
	if isDeno && hasDeps {
		// Deno resolves node_modules relative to file URL — must use relative path from deps dir
		filename := fmt.Sprintf("wkr_%x.js", sha256.Sum256([]byte(code)))
		tmpFile = filepath.Join(depsDir, filename)
		os.WriteFile(tmpFile, []byte(code), 0o644)
		args = []string{"run", "--allow-env", "--allow-read", "--allow-net=0.0.0.0", "--node-modules-dir=auto", filename}
	} else if isDeno {
		args = []string{"eval", "--no-remote", code}
	} else {
		args = []string{"-e", code}
	}

	stdout := e.getBuf()
	stderr := e.getBuf()
	defer e.putBuf(stdout)
	defer e.putBuf(stderr)
	if tmpFile != "" {
		defer os.Remove(tmpFile)
	}

	cmd := exec.CommandContext(ctx, jsBin, args...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = env
	if depsDir != "" {
		cmd.Dir = depsDir
	}

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)
	logs := parseStderrLogs(stderr)

	if ctx.Err() != nil {
		return &ExecutionResult{
			StatusCode: 504,
			Duration:   duration,
			Error:      "execution timed out",
			Logs:       logs,
		}, ErrTimeout
	}

	if err != nil {
		errMsg := extractRuntimeError(stderr.String(), req.Runtime)
		return &ExecutionResult{
			StatusCode: 500,
			Duration:   duration,
			Error:      errMsg,
			Logs:       logs,
		}, nil
	}

	result, parseErr := parseWorkerOutput(copyBytes(stdout.Bytes()), duration)
	result.Logs = logs
	return result, parseErr
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

// Verify performs a dry-run compile/syntax check without executing the worker.
func (e *SandboxEngine) Verify(ctx context.Context, code, runtime, entryPoint string) error {
	return e.VerifyWithVersion(ctx, code, runtime, "", entryPoint, "", "")
}

// VerifyWithVersion performs a dry-run compile/syntax check using a specific runtime version.
func (e *SandboxEngine) VerifyWithVersion(ctx context.Context, code, rt, version, entryPoint, deps, pm string) error {
	switch rt {
	case "go":
		return e.verifyGo(ctx, code, entryPoint, resolveRuntime("go", version))
	case "javascript":
		return e.verifyJS(ctx, code, "js", resolveRuntime("javascript", version), deps, pm)
	case "typescript":
		return e.verifyJS(ctx, code, "ts", resolveRuntime("typescript", version), deps, pm)
	case "python":
		return e.verifyPython(ctx, code, resolveRuntime("python", version))
	default:
		return fmt.Errorf("unsupported runtime: %s", rt)
	}
}

func (e *SandboxEngine) verifyGo(ctx context.Context, code, entryPoint, goBin string) error {
	src := wrapGoCode(code, entryPoint)
	srcPath := filepath.Join(e.goCacheDir, fmt.Sprintf("verify_%x.go", sha256.Sum256([]byte(src))))
	if err := os.WriteFile(srcPath, []byte(src), 0o644); err != nil {
		return fmt.Errorf("write source: %w", err)
	}
	defer os.Remove(srcPath)

	stderr := e.getBuf()
	defer e.putBuf(stderr)

	cmd := exec.CommandContext(ctx, goBin, "vet", srcPath)
	cmd.Stderr = stderr
	cmd.Env = e.hostEnv
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("compilation error:\n%s", stderr.String())
	}
	return nil
}

func (e *SandboxEngine) verifyJS(ctx context.Context, code, ext, jsBin, deps, pm string) error {
	isDeno := strings.Contains(jsBin, "deno")
	hasDeps := strings.TrimSpace(deps) != ""

	// node --check can't parse TypeScript — skip verification when Deno isn't available
	if ext == "ts" && !isDeno {
		return nil
	}

	// If deps are declared, install them first so imports can resolve
	var depsDir string
	if hasDeps {
		dir, err := installDeps(ctx, "javascript", deps, pm, jsBin, e.hostEnv)
		if err != nil {
			return fmt.Errorf("install deps for verify: %w", err)
		}
		depsDir = dir
	}

	// Write code file — into deps dir when deps exist so imports resolve
	writeDir := os.TempDir()
	if depsDir != "" {
		writeDir = depsDir
	}
	filename := fmt.Sprintf("verify_%x.%s", sha256.Sum256([]byte(code)), ext)
	tmpFile := filepath.Join(writeDir, filename)
	os.WriteFile(tmpFile, []byte(code), 0o644)
	defer os.Remove(tmpFile)

	stderr := e.getBuf()
	defer e.putBuf(stderr)

	var cmd *exec.Cmd
	if isDeno {
		if hasDeps {
			// Use relative path from deps dir so Deno resolves deno.json/node_modules
			cmd = exec.CommandContext(ctx, jsBin, "check", "--node-modules-dir=auto", filename)
			cmd.Dir = depsDir
		} else {
			cmd = exec.CommandContext(ctx, jsBin, "check", "--no-remote", tmpFile)
		}
	} else {
		cmd = exec.CommandContext(ctx, jsBin, "--check", tmpFile)
		if depsDir != "" {
			cmd.Env = append(e.hostEnv, "NODE_PATH="+filepath.Join(depsDir, "node_modules"))
		}
	}
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("syntax error:\n%s", stderr.String())
	}
	return nil
}

func (e *SandboxEngine) verifyPython(ctx context.Context, code, pyBin string) error {
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("verify_%x.py", sha256.Sum256([]byte(code))))
	os.WriteFile(tmpFile, []byte(code), 0o644)
	defer os.Remove(tmpFile)

	stderr := e.getBuf()
	defer e.putBuf(stderr)

	cmd := exec.CommandContext(ctx, pyBin, "-m", "py_compile", tmpFile)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("syntax error:\n%s", stderr.String())
	}
	return nil
}

func (e *SandboxEngine) executePython(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	payload := buildWorkerPayload(req)
	code := wrapPyCode(req.Code, req.EntryPoint)

	pyBin := resolveRuntime("python", req.RuntimeVersion)

	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("wkr_%x.py", sha256.Sum256([]byte(code))))
	os.WriteFile(tmpFile, []byte(code), 0o644)
	defer os.Remove(tmpFile)

	env := e.buildWorkerEnv(req.EnvVars)

	// Install pip dependencies if provided
	var execPyBin string = pyBin
	if req.Dependencies != "" {
		depsDir, err := installDeps(ctx, "python", req.Dependencies, req.PackageManager, pyBin, env)
		if err != nil {
			return &ExecutionResult{
				StatusCode: 500,
				Error:      "install deps: " + err.Error(),
			}, nil
		}
		// Use the venv python so native extensions resolve correctly
		venvPython := filepath.Join(depsDir, ".venv", "bin", "python")
		if _, err := os.Stat(venvPython); err == nil {
			execPyBin = venvPython
			env = append(env, "VIRTUAL_ENV="+filepath.Join(depsDir, ".venv"))
		} else {
			env = append(env, "PYTHONPATH="+depsDir)
		}
	}

	stdout := e.getBuf()
	stderr := e.getBuf()
	defer e.putBuf(stdout)
	defer e.putBuf(stderr)

	cmd := exec.CommandContext(ctx, execPyBin, tmpFile)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = env

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)
	logs := parseStderrLogs(stderr)

	if ctx.Err() != nil {
		return &ExecutionResult{
			StatusCode: 504,
			Duration:   duration,
			Error:      "execution timed out",
			Logs:       logs,
		}, ErrTimeout
	}

	if err != nil {
		errMsg := extractRuntimeError(stderr.String(), "python")
		return &ExecutionResult{
			StatusCode: 500,
			Duration:   duration,
			Error:      errMsg,
			Logs:       logs,
		}, nil
	}

	result, parseErr := parseWorkerOutput(copyBytes(stdout.Bytes()), duration)
	result.Logs = logs
	return result, parseErr
}

func wrapPyCode(code, entryPoint string) string {
	return fmt.Sprintf(`import sys, json, os, traceback

# Redirect print() to stderr so it doesn't pollute the JSON response on stdout
print = lambda *a, **kw: __builtins__.__import__('builtins').print(*a, **{**kw, 'file': kw.get('file', sys.stderr)})

def env(key, default=""):
    return os.environ.get(key, default)

def log(*args):
    __builtins__.__import__('builtins').print(*args, file=sys.stderr)

%s

if __name__ == "__main__":
    try:
        payload = json.loads(sys.stdin.read() or "{}")
        result = %s(payload)
        if isinstance(result, dict):
            body = result.get("body", json.dumps(result))
            status = result.get("status", 200)
            headers = result.get("headers", {"Content-Type": "application/json"})
            if not isinstance(body, str):
                body = json.dumps(body)
        else:
            body = json.dumps(result) if result is not None else ""
            status = 200
            headers = {"Content-Type": "application/json"}
        output = json.dumps({"status": status, "headers": headers, "body": body})
        sys.stdout.write(output)
    except Exception:
        traceback.print_exc(file=sys.stderr)
        sys.exit(1)
`, code, entryPoint)
}

func buildWorkerPayload(req *ExecutionRequest) []byte {
	query := make(map[string]string)
	if req.Query != "" {
		if parsed, err := url.ParseQuery(req.Query); err == nil {
			for k := range parsed {
				query[k] = parsed.Get(k)
			}
		}
	}
	p := map[string]interface{}{
		"method":  req.Method,
		"path":    req.Path,
		"query":   query,
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
	// Extract user imports and merge with stdlib
	stdImports := []string{
		"crypto/md5", "crypto/sha256", "encoding/base64", "encoding/hex",
		"encoding/json", "fmt", "io", "net/http", "net/url", "os", "strings", "time",
	}
	userImports, cleanCode := extractGoImports(code)

	// Rename user's entrypoint to avoid conflict with wrapper's main()
	if entryPoint == "main" {
		cleanCode = strings.Replace(cleanCode, "func main(", "func __handler(", 1)
		entryPoint = "__handler"
	}

	seen := make(map[string]struct{})
	for _, i := range stdImports {
		seen[i] = struct{}{}
	}
	for _, i := range userImports {
		if _, ok := seen[i]; !ok {
			stdImports = append(stdImports, i)
			seen[i] = struct{}{}
		}
	}

	var importBlock strings.Builder
	for _, i := range stdImports {
		importBlock.WriteString(fmt.Sprintf("\t%q\n", i))
	}

	return fmt.Sprintf(`package main

import (
%s)

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
	payload, _ := io.ReadAll(os.Stdin)
	var req map[string]interface{}
	json.Unmarshal(payload, &req)
	result := %s(req)
	out, _ := json.Marshal(map[string]interface{}{
		"status":  200,
		"headers": map[string]string{"Content-Type": "application/json"},
		"body":    result,
	})
	fmt.Print(string(out))
}
`, importBlock.String(), cleanCode, entryPoint)
}

// extractGoImports parses import statements from user code and returns them separately.
func extractGoImports(code string) (imports []string, clean string) {
	// Remove "package main" if present
	code = regexp.MustCompile(`(?m)^\s*package\s+main\s*$`).ReplaceAllString(code, "")

	// Match single import: import "pkg"
	single := regexp.MustCompile(`(?m)^\s*import\s+"([^"]+)"\s*$`)
	for _, m := range single.FindAllStringSubmatch(code, -1) {
		imports = append(imports, m[1])
	}
	code = single.ReplaceAllString(code, "")

	// Match import block: import ( ... )
	block := regexp.MustCompile(`(?ms)^\s*import\s*\(\s*(.*?)\s*\)`)
	for _, m := range block.FindAllStringSubmatch(code, -1) {
		for _, line := range strings.Split(m[1], "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Handle aliased imports: alias "pkg"
			if idx := strings.Index(line, `"`); idx >= 0 {
				pkg := strings.Trim(line[idx:], `"`)
				imports = append(imports, pkg)
			}
		}
	}
	code = block.ReplaceAllString(code, "")

	return imports, strings.TrimSpace(code)
}

func wrapJSCode(code, entryPoint string) string {
	// Extract import/require statements from user code and prepend them
	userImports, cleanCode := extractJSImports(code)

	return fmt.Sprintf(`%sconst __cubis={_write(s){try{process.stdout.write(s);return}catch(_){}try{Deno.stdout.writeSync(new TextEncoder().encode(s))}catch(_){}}};
try{console.log=console.warn=console.info=console.debug=(...a)=>console.error(...a)}catch(_){}
async function __readStdin(){try{const b=[];for await(const c of Deno.stdin.readable){b.push(c)}return new TextDecoder().decode(await new Blob(b).arrayBuffer())}catch(_){}try{const fs=require("fs");return fs.readFileSync(0,"utf8")}catch(_){}return"{}"}
function env(k,d){try{const v=Deno.env.get(k);if(v)return v}catch(_){}try{if(process.env[k])return process.env[k]}catch(_){}return d!==undefined?d:""}
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

;(async()=>{const payload=jsonParse(await __readStdin(),{});let result=%s(payload);if(result instanceof Promise)result=await result;const output=jsonStringify({status:200,headers:{"Content-Type":"application/json"},body:typeof result==="string"?result:jsonStringify(result)});__cubis._write(output)})();
`, userImports, cleanCode, entryPoint)
}

// extractJSImports pulls import/require statements to the top (ES modules require imports at top level).
func extractJSImports(code string) (imports string, clean string) {
	var importLines []string
	var codeLines []string
	for _, line := range strings.Split(code, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "import{") {
			importLines = append(importLines, line)
		} else if strings.HasPrefix(trimmed, "const ") && strings.Contains(trimmed, "require(") {
			importLines = append(importLines, line)
		} else {
			codeLines = append(codeLines, line)
		}
	}
	if len(importLines) > 0 {
		return strings.Join(importLines, "\n") + "\n", strings.Join(codeLines, "\n")
	}
	return "", code
}

// parseStderrLogs splits stderr into non-empty lines for structured log capture.
// ANSI escape codes are stripped for clean JSON output.
func parseStderrLogs(buf *bytes.Buffer) []string {
	if buf.Len() == 0 {
		return nil
	}
	raw := stripANSI(strings.TrimSpace(buf.String()))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// extractRuntimeError parses stderr to find the actual error message
// instead of returning useless "exit status 1".
func extractRuntimeError(stderr, rt string) string {
	stderr = stripANSI(strings.TrimSpace(stderr))
	if stderr == "" {
		return "worker exited with error (no output)"
	}

	lines := strings.Split(stderr, "\n")

	// For JS/TS: look for "Uncaught", "Error:", "TypeError:", "RangeError:", etc.
	if rt == "javascript" || rt == "typescript" {
		for i := len(lines) - 1; i >= 0; i-- {
			l := strings.TrimSpace(lines[i])
			for _, prefix := range []string{"Uncaught", "Error:", "TypeError:", "RangeError:", "ReferenceError:", "SyntaxError:"} {
				if strings.Contains(l, prefix) {
					// Return this line plus any following stack lines (up to 5)
					end := i + 6
					if end > len(lines) {
						end = len(lines)
					}
					return strings.Join(lines[i:end], "\n")
				}
			}
		}
	}

	// For Go: look for "panic:", "runtime error:", or the last non-empty lines
	if rt == "go" {
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "panic:") || strings.Contains(l, "runtime error:") {
				return l
			}
		}
	}

	// For Python: look for the last "Error:" or "Traceback" block
	if rt == "python" {
		for i := len(lines) - 1; i >= 0; i-- {
			l := strings.TrimSpace(lines[i])
			for _, suffix := range []string{"Error:", "Exception:"} {
				if strings.Contains(l, suffix) {
					return l
				}
			}
		}
	}

	// Fallback: return last meaningful lines (up to 10)
	start := 0
	if len(lines) > 10 {
		start = len(lines) - 10
	}
	return strings.Join(lines[start:], "\n")
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
