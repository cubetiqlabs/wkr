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

	payload := map[string]interface{}{
		"name":        cfg.Name,
		"runtime":     cfg.Runtime,
		"entry_point": cfg.EntryPoint,
		"code":        string(code),
	}
	if len(cfg.EnvVars) > 0 {
		payload["env_vars"] = cfg.EnvVars
	}

	fmt.Printf("Deploying %s (%s)...\n", cfg.Name, cfg.Runtime)

	// Try update first (PUT), fall back to create (POST)
	url := creds.APIURL + "/api/v1/workers"
	resp, err := apiRequest("PUT", url+"/by-name/"+cfg.Name, creds.Token, map[string]interface{}{
		"code":        string(code),
		"entry_point": cfg.EntryPoint,
		"env_vars":    cfg.EnvVars,
	})

	action := "updated"
	if err != nil || !resp.Success {
		if err == nil && resp.Error != "" && resp.Error != "worker not found" {
			// Real error (verification failed, etc.) — don't fall through to create
			fatal("deploy failed: " + resp.Error)
		}
		// Worker doesn't exist yet — create it
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
