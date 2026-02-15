package main

import (
	"encoding/json"
	"fmt"
)

func cmdList() {
	creds, err := loadCredentials()
	if err != nil {
		fatal(err.Error())
	}

	resp, err := apiRequest("GET", creds.APIURL+"/api/v1/workers", creds.Token, nil)
	if err != nil {
		fatal(err.Error())
	}
	if !resp.Success {
		fatal(resp.Error)
	}

	var workers []struct {
		Name    string `json:"name"`
		Runtime string `json:"runtime"`
		Version int    `json:"version"`
		Status  string `json:"status"`
	}
	json.Unmarshal(resp.Data, &workers)

	if len(workers) == 0 {
		fmt.Println("No workers deployed yet.")
		return
	}

	fmt.Printf("%-25s %-12s %-8s %s\n", "NAME", "RUNTIME", "VERSION", "STATUS")
	for _, w := range workers {
		fmt.Printf("%-25s %-12s v%-7d %s\n", w.Name, w.Runtime, w.Version, w.Status)
	}
}
