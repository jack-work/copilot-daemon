package main

import (
	"fmt"
	"os"
)

const version = "0.2.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "start":
		if err := runDaemon(); err != nil {
			fmt.Fprintf(os.Stderr, "daemon error: %v\n", err)
			os.Exit(1)
		}
	case "install":
		if err := install(); err != nil {
			fmt.Fprintf(os.Stderr, "install error: %v\n", err)
			os.Exit(1)
		}
	case "uninstall":
		if err := uninstall(); err != nil {
			fmt.Fprintf(os.Stderr, "uninstall error: %v\n", err)
			os.Exit(1)
		}
	case "status":
		if err := status(); err != nil {
			fmt.Fprintf(os.Stderr, "status error: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println("copilot-daemon", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`copilot-daemon - process supervisor for copilot-api

Usage:
  copilot-daemon <command>

Commands:
  start       Run the daemon (foreground). Supervises copilot-api with auto-restart.
  install     Register as a Windows scheduled task and start immediately.
  uninstall   Stop and remove the scheduled task.
  status      Show whether the daemon and copilot-api are running.
  version     Print version.
  help        Print this message.`)
}
