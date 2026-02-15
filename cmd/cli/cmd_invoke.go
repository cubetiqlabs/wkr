package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func cmdInvoke() {
	if len(os.Args) < 3 {
		fatal("usage: wkr-cli invoke <worker-name> [json-body]")
	}

	creds, err := loadCredentials()
	if err != nil {
		fatal(err.Error())
	}

	name := os.Args[2]
	var body interface{}
	if len(os.Args) > 3 {
		json.Unmarshal([]byte(strings.Join(os.Args[3:], " ")), &body)
	}

	resp, err := apiRequest("POST", creds.APIURL+"/api/v1/invoke/"+name, "", body)
	if err != nil {
		fatal(err.Error())
	}

	out, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Println(string(out))
}
