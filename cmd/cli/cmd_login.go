package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"
)

func cmdLogin() {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	apiURL := fs.String("api-url", "http://localhost:8080", "Cubis Workers API URL")
	email := fs.String("email", "", "Account email")
	password := fs.String("password", "", "Account password (omit for interactive prompt)")
	fs.Parse(os.Args[2:])

	if *email == "" {
		fmt.Print("Email: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			*email = strings.TrimSpace(scanner.Text())
		}
	}

	if *password == "" {
		fmt.Print("Password: ")
		raw, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			fatal("failed to read password")
		}
		*password = string(raw)
	}

	resp, err := apiRequest("POST", *apiURL+"/api/v1/auth/login", "", map[string]string{
		"email":    *email,
		"password": *password,
	})
	if err != nil {
		fatal(err.Error())
	}
	if !resp.Success {
		fatal("login failed: " + resp.Error)
	}

	var data struct {
		Token string `json:"token"`
		User  struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	json.Unmarshal(resp.Data, &data)

	if err := saveCredentials(&Credentials{
		APIURL:   strings.TrimRight(*apiURL, "/"),
		Token:    data.Token,
		Email:    *email,
		Username: data.User.Username,
	}); err != nil {
		fatal("failed to save credentials: " + err.Error())
	}

	fmt.Println("✓ Logged in as", *email)
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(1)
}
