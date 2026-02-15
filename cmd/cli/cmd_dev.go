package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func cmdDev() {
	fs := flag.NewFlagSet("dev", flag.ExitOnError)
	body := fs.String("body", "", "JSON request body")
	method := fs.String("method", "GET", "HTTP method")
	path := fs.String("path", "/", "Request path")
	query := fs.String("query", "", "Query string (e.g. foo=bar&page=2)")
	fs.Parse(os.Args[2:])

	cfg := loadConfig()

	code, err := os.ReadFile(cfg.Main)
	if err != nil {
		fatal("cannot read source file: " + cfg.Main)
	}

	// Build the same payload the server sends to workers
	queryMap := make(map[string]string)
	if *query != "" {
		for _, pair := range strings.Split(*query, "&") {
			if k, v, ok := strings.Cut(pair, "="); ok {
				queryMap[k] = v
			}
		}
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"method":  *method,
		"path":    *path,
		"query":   queryMap,
		"headers": map[string]string{},
		"body":    *body,
	})

	var out []byte
	switch cfg.Runtime {
	case "go":
		out, err = runGoLocal(string(code), cfg.EntryPoint, cfg.EnvVars, payload)
	case "javascript", "typescript":
		out, err = runJSLocal(string(code), cfg.EntryPoint, cfg.EnvVars, payload)
	case "python":
		out, err = runPyLocal(string(code), cfg.EntryPoint, cfg.EnvVars, payload)
	default:
		fatal("unsupported runtime: " + cfg.Runtime)
	}
	if err != nil {
		fatal(err.Error())
	}

	// Pretty-print if JSON
	var parsed json.RawMessage
	if json.Unmarshal(out, &parsed) == nil {
		pretty, _ := json.MarshalIndent(parsed, "", "  ")
		fmt.Println(string(pretty))
	} else {
		fmt.Print(string(out))
	}
}

func runJSLocal(code, entryPoint string, envVars map[string]string, payload []byte) ([]byte, error) {
	// Detect runtime
	rt := "node"
	if _, err := exec.LookPath("deno"); err == nil {
		rt = "deno"
	}

	// Minimal wrapper: read stdin, call entrypoint, write result
	wrapped := fmt.Sprintf(`
try{console.log=console.warn=console.info=console.debug=(...a)=>console.error(...a)}catch(_){}
async function __readStdin(){try{const b=[];for await(const c of Deno.stdin.readable){b.push(c)}return new TextDecoder().decode(await new Blob(b).arrayBuffer())}catch(_){}try{const fs=require("fs");return fs.readFileSync(0,"utf8")}catch(_){}return"{}"}
function env(k,d){try{const v=Deno.env.get(k);if(v)return v}catch(_){}try{if(process.env[k])return process.env[k]}catch(_){}return d!==undefined?d:""}
function log(...a){console.error(...a)}
function jsonParse(s,f){try{return JSON.parse(s)}catch(_){return f!==undefined?f:null}}

%s

;(async()=>{const payload=jsonParse(await __readStdin(),{});let result=%s(payload);if(result instanceof Promise)result=await result;const output=JSON.stringify(result);try{process.stdout.write(output)}catch(_){try{Deno.stdout.writeSync(new TextEncoder().encode(output))}catch(_){}}})();
`, code, entryPoint)

	var args []string
	if rt == "deno" {
		args = []string{"eval", "--no-remote", wrapped}
	} else {
		tmp := filepath.Join(os.TempDir(), "wkr-dev.mjs")
		os.WriteFile(tmp, []byte(wrapped), 0644)
		defer os.Remove(tmp)
		args = []string{tmp}
	}

	cmd := exec.Command(rt, args...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = buildLocalEnv(envVars)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%s", stderr.String())
		}
		return nil, err
	}

	if stderr.Len() > 0 {
		fmt.Fprint(os.Stderr, stderr.String())
	}
	return stdout.Bytes(), nil
}

func runGoLocal(code, entryPoint string, envVars map[string]string, payload []byte) ([]byte, error) {
	// Write wrapped source to temp file and go run it
	wrapped := wrapGoLocal(code, entryPoint)
	tmp := filepath.Join(os.TempDir(), "wkr-dev.go")
	if err := os.WriteFile(tmp, []byte(wrapped), 0644); err != nil {
		return nil, err
	}
	defer os.Remove(tmp)

	cmd := exec.Command("go", "run", tmp)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = buildLocalEnv(envVars)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%s", stderr.String())
		}
		return nil, err
	}

	if stderr.Len() > 0 {
		fmt.Fprint(os.Stderr, stderr.String())
	}
	return stdout.Bytes(), nil
}

func wrapGoLocal(code, entryPoint string) string {
	userImports, cleanCode := extractGoUserImports(code)

	// Rename user's entrypoint to avoid conflict with wrapper's main()
	if entryPoint == "main" {
		cleanCode = strings.Replace(cleanCode, "func main(", "func __handler(", 1)
		entryPoint = "__handler"
	}

	stdImports := []string{"encoding/json", "fmt", "io", "os"}
	seen := map[string]struct{}{"encoding/json": {}, "fmt": {}, "io": {}, "os": {}}
	for _, i := range userImports {
		if _, ok := seen[i]; !ok {
			stdImports = append(stdImports, i)
			seen[i] = struct{}{}
		}
	}

	var ib strings.Builder
	for _, i := range stdImports {
		ib.WriteString(fmt.Sprintf("\t%q\n", i))
	}

	return fmt.Sprintf(`package main

import (
%s)

func Log(args ...interface{}) { fmt.Fprintln(os.Stderr, args...) }
func Env(key string) string { return os.Getenv(key) }

%s

func main() {
	payload, _ := io.ReadAll(os.Stdin)
	var req map[string]interface{}
	json.Unmarshal(payload, &req)
	result := %s(req)
	out, _ := json.Marshal(result)
	fmt.Print(string(out))
}
`, ib.String(), cleanCode, entryPoint)
}

func extractGoUserImports(code string) ([]string, string) {
	var imports []string
	var lines []string
	inBlock := false
	for _, line := range strings.Split(code, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "package main" {
			continue
		}
		if strings.HasPrefix(trimmed, "import (") {
			inBlock = true
			continue
		}
		if inBlock {
			if trimmed == ")" {
				inBlock = false
				continue
			}
			if idx := strings.Index(trimmed, `"`); idx >= 0 {
				imports = append(imports, strings.Trim(trimmed[idx:], `"`))
			}
			continue
		}
		if strings.HasPrefix(trimmed, `import "`) {
			pkg := strings.TrimPrefix(trimmed, `import "`)
			pkg = strings.TrimSuffix(pkg, `"`)
			imports = append(imports, pkg)
			continue
		}
		lines = append(lines, line)
	}
	return imports, strings.Join(lines, "\n")
}

func buildLocalEnv(envVars map[string]string) []string {
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, k+"="+v)
	}
	return env
}

func runPyLocal(code, entryPoint string, envVars map[string]string, payload []byte) ([]byte, error) {
	wrapped := fmt.Sprintf(`import sys, json, os

print = lambda *a, **kw: __builtins__.__import__('builtins').print(*a, **{**kw, 'file': kw.get('file', sys.stderr)})

def env(key, default=""):
    return os.environ.get(key, default)

def log(*args):
    __builtins__.__import__('builtins').print(*args, file=sys.stderr)

%s

payload = json.loads(sys.stdin.read() or "{}")
result = %s(payload)
output = json.dumps(result)
sys.stdout.write(output)
`, code, entryPoint)

	tmp := filepath.Join(os.TempDir(), "wkr-dev.py")
	os.WriteFile(tmp, []byte(wrapped), 0644)
	defer os.Remove(tmp)

	cmd := exec.Command("python3", tmp)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = buildLocalEnv(envVars)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%s", stderr.String())
		}
		return nil, err
	}

	if stderr.Len() > 0 {
		fmt.Fprint(os.Stderr, stderr.String())
	}
	return stdout.Bytes(), nil
}
