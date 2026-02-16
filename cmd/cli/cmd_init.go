package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

type WorkerConfig struct {
	Name           string            `yaml:"name"`
	Runtime        string            `yaml:"runtime"`
	RuntimeVersion string            `yaml:"runtime_version,omitempty"`
	EntryPoint     string            `yaml:"entry_point"`
	Main           string            `yaml:"main"`
	PackageManager string            `yaml:"package_manager,omitempty"` // npm, yarn, pnpm, deno, pip, uv, go (auto-detected if empty)
	EnvVars        map[string]string `yaml:"env_vars,omitempty"`
}

const configFile = "wkr.yaml"

var templates = map[string]struct {
	runtime string
	code    string
}{
	"hello-js": {"javascript", `function main(req) {
  return { message: "Hello from Cubis Workers!" };
}
`},
	"hello-ts": {"typescript", `function main(req: any): any {
  return { message: "Hello from Cubis Workers!" };
}
`},
	"hello-go": {"go", `package main

func main(req map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"message": "Hello from Cubis Workers!",
	}
}
`},
	"json-api": {"javascript", `function main(req) {
  const body = JSON.parse(req.body || "{}");
  return {
    status: 200,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ok: true, received: body }),
  };
}
`},
	"cron": {"javascript", `function main(req) {
  const now = new Date().toISOString();
  console.log("cron tick at", now);
  return { executed_at: now };
}
`},
	"proxy": {"javascript", `function main(req) {
  const target = req.headers["X-Target-Url"] || "https://httpbin.org/get";
  return {
    proxy: target,
    headers: { "X-Forwarded-By": "cubis-worker" },
  };
}
`},
	"hello-py": {"python", `def main(req):
    return {"message": "Hello from Cubis Workers!"}
`},
}

func cmdInit() {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	name := fs.String("name", "", "Worker name (creates subfolder if set)")
	runtime := fs.String("runtime", "", "Runtime: go, javascript, typescript, python")
	version := fs.String("runtime-version", "", "Runtime version: e.g. 1.24, 3.12, 22")
	tmpl := fs.String("template", "", "Template: hello-js, hello-ts, hello-go, json-api, cron, proxy")
	listTmpl := fs.Bool("list-templates", false, "List available templates")
	fs.Parse(os.Args[2:])

	if *listTmpl {
		fmt.Println("Available templates:")
		fmt.Printf("\n  %-14s %s\n", "NAME", "RUNTIME")
		for k, v := range templates {
			fmt.Printf("  %-14s %s\n", k, v.runtime)
		}
		return
	}

	// Resolve template
	var code string
	if *tmpl != "" {
		t, ok := templates[*tmpl]
		if !ok {
			fatal("unknown template: " + *tmpl + " (use --list-templates)")
		}
		if *runtime == "" {
			*runtime = t.runtime
		}
		code = t.code
	}
	if *runtime == "" {
		*runtime = "javascript"
	}

	// If --name is set, create and cd into subfolder
	if *name != "" {
		if err := os.MkdirAll(*name, 0755); err != nil {
			fatal("failed to create directory: " + err.Error())
		}
		if err := os.Chdir(*name); err != nil {
			fatal("failed to enter directory: " + err.Error())
		}
		fmt.Println("✓ Created", *name+"/")
	} else {
		*name = filepath.Base(mustCwd())
	}

	ext := map[string]string{"go": ".go", "javascript": ".js", "typescript": ".ts", "python": ".py"}
	mainFile := "worker" + ext[*runtime]

	cfg := WorkerConfig{
		Name:           *name,
		Runtime:        *runtime,
		RuntimeVersion: *version,
		EntryPoint:     "main",
		Main:           mainFile,
	}

	data, _ := yaml.Marshal(cfg)
	// Prepend schema comment for editor auto-completion
	content := "# yaml-language-server: $schema=https://raw.githubusercontent.com/cubetiqlabs/wkr/main/schemas/wkr.schema.json\n" + string(data)
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		fatal("failed to write " + configFile + ": " + err.Error())
	}

	if _, err := os.Stat(mainFile); os.IsNotExist(err) {
		if code == "" {
			code = scaffold(*runtime)
		}
		os.WriteFile(mainFile, []byte(code), 0644)
		fmt.Println("✓ Created", mainFile)
	}

	fmt.Println("✓ Initialized", configFile)
}

func loadConfig() *WorkerConfig {
	data, err := os.ReadFile(configFile)
	if err != nil {
		fatal("no wkr.yaml found — run: wkr-cli init")
	}
	var cfg WorkerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fatal("invalid wkr.yaml: " + err.Error())
	}
	return &cfg
}

func mustCwd() string {
	dir, err := os.Getwd()
	if err != nil {
		fatal("cannot get working directory")
	}
	return dir
}

func scaffold(runtime string) string {
	switch runtime {
	case "go":
		return templates["hello-go"].code
	case "typescript":
		return templates["hello-ts"].code
	case "python":
		return templates["hello-py"].code
	default:
		return templates["hello-js"].code
	}
}
