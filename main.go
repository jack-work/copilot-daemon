package main

import (
	"fmt"
	"os"
)

const version = "0.3.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "start":
		foreground := len(os.Args) > 2 && os.Args[2] == "--foreground"
		if foreground {
			err = runDaemon()
		} else {
			err = daemonize()
		}
	case "install":
		err = install()
	case "uninstall":
		err = uninstall()
	case "status":
		err = queryStatus()
	case "stop":
		err = stopDaemon()
	case "logs":
		err = tailLogs()
	case "version":
		fmt.Println("copilot-daemon", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`copilot-daemon - process supervisor for copilot-api

Usage:
  copilot-daemon <command>

Commands:
  start             Detach and run the daemon in the background (no window).
  start --foreground  Run in foreground (used by Task Scheduler).
  install           Register as a Windows scheduled task and start.
  uninstall         Stop and remove the scheduled task.
  status            Query the running daemon for status.
  stop              Gracefully stop the running daemon.
  logs              Tail the daemon's log stream (Ctrl+C to stop).
  version           Print version.
  help              Print this message.`)
}
