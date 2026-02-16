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
		Name           string `json:"name"`
		Runtime        string `json:"runtime"`
		RuntimeVersion string `json:"runtime_version"`
		EntryPoint     string `json:"entry_point"`
		CodeHash       string `json:"code_hash"`
		PackageManager string `json:"package_manager"`
		Version        int    `json:"version"`
		Status         string `json:"status"`
		MemoryLimit    int    `json:"memory_limit"`
		UpdatedAt      string `json:"updated_at"`
	}
	json.Unmarshal(resp.Data, &worker)

	fmt.Printf("\n✓ Worker %s %s\n\n", worker.Name, action)

	invokeURL := creds.APIURL + "/api/v1/invoke/"
	if creds.Username != "" {
		invokeURL += "@" + creds.Username + "/"
	}
	invokeURL += cfg.Name

	rt = worker.Runtime
	if worker.RuntimeVersion != "" {
		rt += " " + worker.RuntimeVersion
	}

	str := "  %-16s %s\n"
	fmt.Printf(str, "Name:", worker.Name)
	fmt.Printf(str, "Version:", fmt.Sprintf("v%d", worker.Version))
	fmt.Printf(str, "Status:", worker.Status)
	fmt.Printf(str, "Runtime:", rt)
	fmt.Printf(str, "Entry point:", worker.EntryPoint)
	fmt.Printf(str, "Code hash:", worker.CodeHash[:12])
	if worker.PackageManager != "" {
		fmt.Printf(str, "Package mgr:", worker.PackageManager)
	}
	fmt.Printf(str, "Memory limit:", fmt.Sprintf("%d MB", worker.MemoryLimit))
	fmt.Printf(str, "Deployed at:", worker.UpdatedAt)
	fmt.Printf("\n"+str, "Invoke URL:", invokeURL)
}
