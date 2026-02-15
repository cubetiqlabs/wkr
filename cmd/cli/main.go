package main

import (
	"fmt"
	"os"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "login":
		cmdLogin()
	case "init":
		cmdInit()
	case "deploy":
		cmdDeploy()
	case "list", "ls":
		cmdList()
	case "invoke":
		cmdInvoke()
	case "delete", "rm":
		cmdDelete()
	case "version", "-v", "--version":
		fmt.Println("wkr-cli v" + version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`wkr-cli — Cubis Workers CLI

Usage: wkr-cli <command> [options]

Commands:
  login          Authenticate with the Cubis Workers API
  init           Initialize a new worker project (creates wkr.yaml)
  deploy         Deploy the current worker to the platform
  list, ls       List your deployed workers
  invoke         Invoke a worker by name
  delete, rm     Delete a worker by name
  version        Print CLI version
  help           Show this help message

Examples:
  wkr-cli login --api-url http://localhost:8080 --email dev@example.com
  wkr-cli init --name hello --runtime javascript
  wkr-cli deploy
  wkr-cli invoke hello
  wkr-cli list`)
}
