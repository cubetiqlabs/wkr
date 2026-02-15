package main

import "fmt"

func cmdWhoami() {
	creds, err := loadCredentials()
	if err != nil {
		fatal(err.Error())
	}

	fmt.Printf("Email:    %s\n", creds.Email)
	fmt.Printf("Username: %s\n", creds.Username)
	fmt.Printf("API URL:  %s\n", creds.APIURL)
}
