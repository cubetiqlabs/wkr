package main

import (
	"fmt"
	"os"
)

func cmdDelete() {
	if len(os.Args) < 3 {
		fatal("usage: wkr-cli delete <worker-name>")
	}

	creds, err := loadCredentials()
	if err != nil {
		fatal(err.Error())
	}

	name := os.Args[2]

	// Get worker ID by listing and matching name
	resp, err := apiRequest("DELETE", creds.APIURL+"/api/v1/workers/by-name/"+name, creds.Token, nil)
	if err != nil {
		fatal(err.Error())
	}
	if !resp.Success {
		fatal("delete failed: " + resp.Error)
	}

	fmt.Println("✓ Deleted worker:", name)
}
