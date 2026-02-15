package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func cmdRollback() {
	fs := flag.NewFlagSet("rollback", flag.ExitOnError)
	version := fs.Int("version", 0, "Target version to rollback to (required)")
	fs.Parse(os.Args[2:])

	creds, err := loadCredentials()
	if err != nil {
		fatal(err.Error())
	}

	var name string
	if fs.NArg() > 0 {
		name = fs.Arg(0)
	} else {
		cfg := loadConfig()
		name = cfg.Name
	}

	if *version < 1 {
		fatal("usage: wkr-cli rollback [worker-name] --version <N>")
	}

	fmt.Printf("Rolling back %s to v%d...\n", name, *version)

	resp, err := apiRequest("POST", creds.APIURL+"/api/v1/workers/by-name/"+name+"/rollback", creds.Token, map[string]int{
		"version": *version,
	})
	if err != nil {
		fatal(err.Error())
	}
	if !resp.Success {
		fatal("rollback failed: " + resp.Error)
	}

	var worker struct {
		Name    string `json:"name"`
		Version int    `json:"version"`
		Status  string `json:"status"`
	}
	json.Unmarshal(resp.Data, &worker)

	fmt.Printf("✓ Rolled back %s to v%d (now at v%d, %s)\n", worker.Name, *version, worker.Version, worker.Status)
}
