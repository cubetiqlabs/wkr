package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func cmdDeploy() {
	creds, err := loadCredentials()
	if err != nil {
		fatal(err.Error())
	}
	cfg := loadConfig()

	code, err := os.ReadFile(cfg.Main)
	if err != nil {
		fatal("cannot read source file: " + cfg.Main)
	}

	// Auto-detect and install dependencies
	deps, _ := installLocalDeps(cfg)

	pm := detectPackageManager(cfg)

	payload := map[string]interface{}{
		"name":            cfg.Name,
		"runtime":         cfg.Runtime,
		"runtime_version": cfg.RuntimeVersion,
		"entry_point":     cfg.EntryPoint,
		"code":            string(code),
		"dependencies":    deps,
		"package_manager": pm,
	}
	if len(cfg.EnvVars) > 0 {
		payload["env_vars"] = cfg.EnvVars
	}

	rt := cfg.Runtime
	if cfg.RuntimeVersion != "" {
		rt += " " + cfg.RuntimeVersion
	}
	fmt.Printf("Deploying %s (%s)...\n", cfg.Name, rt)

	// Try update first (PUT), fall back to create (POST)
	url := creds.APIURL + "/api/v1/workers"
	resp, err := apiRequest("PUT", url+"/by-name/"+cfg.Name, creds.Token, map[string]interface{}{
		"runtime":         cfg.Runtime,
		"runtime_version": cfg.RuntimeVersion,
		"code":            string(code),
		"entry_point":     cfg.EntryPoint,
		"dependencies":    deps,
		"package_manager": pm,
		"env_vars":        cfg.EnvVars,
	})

	action := "updated"
	if err != nil || !resp.Success {
		if err == nil && resp.Error != "" && resp.Error != "worker not found" {
			fatal("deploy failed: " + resp.Error)
		}
		resp, err = apiRequest("POST", url, creds.Token, payload)
		if err != nil {
			fatal(err.Error())
		}
		if !resp.Success {
			fatal("deploy failed: " + resp.Error)
		}
		action = "created"
	}

	var worker struct {
		Name    string `json:"name"`
		Version int    `json:"version"`
		Status  string `json:"status"`
	}
	json.Unmarshal(resp.Data, &worker)

	fmt.Printf("✓ Worker %s %s (v%d, %s)\n", worker.Name, action, worker.Version, worker.Status)
	if creds.Username != "" {
		fmt.Printf("→ Invoke: %s/api/v1/invoke/@%s/%s\n", creds.APIURL, creds.Username, cfg.Name)
	} else {
		fmt.Printf("→ Invoke: %s/api/v1/invoke/%s\n", creds.APIURL, cfg.Name)
	}
}
