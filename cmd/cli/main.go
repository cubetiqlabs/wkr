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
	case "logout":
		cmdLogout()
	case "whoami":
		cmdWhoami()
	case "init":
		cmdInit()
	case "deploy":
		cmdDeploy()
	case "dev":
		cmdDev()
	case "list", "ls":
		cmdList()
	case "invoke":
		cmdInvoke()
	case "delete", "rm":
		cmdDelete()
	case "revisions", "rev":
		cmdRevisions()
	case "logs":
		cmdLogs()
	case "rollback":
		cmdRollback()
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
  login              Authenticate with the Cubis Workers API
  logout             Remove stored credentials
  whoami             Show current authenticated user
  init               Initialize a new worker project
  deploy             Deploy the current worker to the platform
  dev                Run worker locally (no deploy needed)
  list, ls           List your deployed workers
  invoke             Invoke a worker by name
  delete, rm         Delete a worker by name
  revisions, rev     List deployment revisions for a worker
  logs               View invocation logs for a worker
  rollback           Rollback a worker to a specific version
  version            Print CLI version
  help               Show this help message

Init options:
  --name <name>      Worker name (creates subfolder if set)
  --runtime <rt>     Runtime: go, javascript, typescript (default: javascript)
  --template <tpl>   Use a prebuilt template (see --list-templates)
  --list-templates   List available templates

Examples:
  wkr-cli login --api-url http://localhost:8080 --email dev@example.com
  wkr-cli whoami
  wkr-cli init --name my-api --template json-api
  wkr-cli init --template hello-go
  wkr-cli deploy
  wkr-cli dev --body '{"name":"test"}' --query 'page=1'
  wkr-cli invoke hello
  wkr-cli revisions hello
  wkr-cli logs hello
  wkr-cli logs hello -f
  wkr-cli rollback hello --version 2
  wkr-cli logout`)
}
