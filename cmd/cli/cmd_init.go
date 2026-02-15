package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

type WorkerConfig struct {
	Name       string            `yaml:"name"`
	Runtime    string            `yaml:"runtime"`
	EntryPoint string            `yaml:"entry_point"`
	Main       string            `yaml:"main"` // source file path
	EnvVars    map[string]string `yaml:"env_vars,omitempty"`
}

const configFile = "wkr.yaml"

func cmdInit() {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	name := fs.String("name", "", "Worker name")
	runtime := fs.String("runtime", "javascript", "Runtime: go, javascript, typescript")
	fs.Parse(os.Args[2:])

	if *name == "" {
		*name = filepath.Base(mustCwd())
	}

	ext := map[string]string{"go": ".go", "javascript": ".js", "typescript": ".ts"}
	mainFile := "worker" + ext[*runtime]

	cfg := WorkerConfig{
		Name:       *name,
		Runtime:    *runtime,
		EntryPoint: "main",
		Main:       mainFile,
	}

	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		fatal("failed to write " + configFile + ": " + err.Error())
	}

	// Create scaffold source file if it doesn't exist
	if _, err := os.Stat(mainFile); os.IsNotExist(err) {
		os.WriteFile(mainFile, []byte(scaffold(*runtime)), 0644)
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
		return `package main

func main(req map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"message": "Hello from Cubis Workers!",
	}
}
`
	case "typescript":
		return `function main(req: any): any {
  return { message: "Hello from Cubis Workers!" };
}
`
	default:
		return `function main(req) {
  return { message: "Hello from Cubis Workers!" };
}
`
	}
}
