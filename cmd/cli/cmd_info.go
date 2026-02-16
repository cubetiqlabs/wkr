package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func cmdInfo() {
	creds, err := loadCredentials()
	if err != nil {
		fatal(err.Error())
	}

	// Use arg or wkr.yaml name
	name := ""
	if len(os.Args) > 2 {
		name = os.Args[2]
	} else {
		cfg := loadConfig()
		name = cfg.Name
	}

	resp, err := apiRequest("GET", creds.APIURL+"/api/v1/workers/by-name/"+name, creds.Token, nil)
	if err != nil {
		fatal(err.Error())
	}
	if !resp.Success {
		fatal(resp.Error)
	}

	var w struct {
		Name           string `json:"name"`
		Runtime        string `json:"runtime"`
		RuntimeVersion string `json:"runtime_version"`
		EntryPoint     string `json:"entry_point"`
		CodeHash       string `json:"code_hash"`
		PackageManager string `json:"package_manager"`
		Version        int    `json:"version"`
		Status         string `json:"status"`
		MemoryLimit    int    `json:"memory_limit"`
		Timeout        int64  `json:"timeout"`
		CreatedAt      string `json:"created_at"`
		UpdatedAt      string `json:"updated_at"`
	}
	json.Unmarshal(resp.Data, &w)

	rt := w.Runtime
	if w.RuntimeVersion != "" {
		rt += " " + w.RuntimeVersion
	}

	invokeURL := creds.APIURL + "/api/v1/invoke/"
	if creds.Username != "" {
		invokeURL += "@" + creds.Username + "/"
	}
	invokeURL += name

	str := "%-16s %s\n"
	fmt.Printf(str, "Name:", w.Name)
	fmt.Printf(str, "Version:", fmt.Sprintf("v%d", w.Version))
	fmt.Printf(str, "Status:", w.Status)
	fmt.Printf(str, "Runtime:", rt)
	fmt.Printf(str, "Entry point:", w.EntryPoint)
	fmt.Printf(str, "Code hash:", w.CodeHash[:min(12, len(w.CodeHash))])
	if w.PackageManager != "" {
		fmt.Printf(str, "Package mgr:", w.PackageManager)
	}
	fmt.Printf(str, "Memory limit:", fmt.Sprintf("%d MB", w.MemoryLimit))
	fmt.Printf(str, "Timeout:", fmt.Sprintf("%dms", w.Timeout/1e6))
	fmt.Printf(str, "Created at:", w.CreatedAt)
	fmt.Printf(str, "Deployed at:", w.UpdatedAt)
	fmt.Printf("\n%-16s %s\n", "Invoke URL:", invokeURL)
}
