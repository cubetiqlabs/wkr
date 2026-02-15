package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func cmdRevisions() {
	creds, err := loadCredentials()
	if err != nil {
		fatal(err.Error())
	}

	var name string
	if len(os.Args) > 2 {
		name = os.Args[2]
	} else {
		cfg := loadConfig()
		name = cfg.Name
	}

	resp, err := apiRequest("GET", creds.APIURL+"/api/v1/workers/by-name/"+name+"/revisions", creds.Token, nil)
	if err != nil {
		fatal(err.Error())
	}
	if !resp.Success {
		fatal(resp.Error)
	}

	var revisions []struct {
		Version    int       `json:"version"`
		CodeHash   string    `json:"code_hash"`
		EntryPoint string    `json:"entry_point"`
		Status     string    `json:"status"`
		CreatedAt  time.Time `json:"created_at"`
	}
	json.Unmarshal(resp.Data, &revisions)

	if len(revisions) == 0 {
		fmt.Println("No revisions found.")
		return
	}

	fmt.Printf("Revisions for %s:\n\n", name)
	fmt.Printf("%-8s %-12s %-12s %-10s %s\n", "VERSION", "STATUS", "ENTRY", "HASH", "DEPLOYED AT")
	for _, r := range revisions {
		fmt.Printf("v%-7d %-12s %-12s %.8s   %s\n", r.Version, r.Status, r.EntryPoint, r.CodeHash, r.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	}
}
