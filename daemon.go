package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const (
	initialBackoff = 2 * time.Second
	maxBackoff     = 60 * time.Second
	backoffFactor  = 2
	maxLogSize     = 10 * 1024 * 1024 // 10MB
)

func logDir() string {
	exe, _ := os.Executable()
	return filepath.Join(filepath.Dir(exe), "logs")
}

func logFile() string {
	return filepath.Join(logDir(), "copilot-daemon.log")
}

func rotateLog(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxLogSize {
		return
	}
	prev := path + ".prev"
	os.Remove(prev)
	os.Rename(path, prev)
}

func openLog() (*os.File, error) {
	dir := logDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	path := logFile()
	rotateLog(path)
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
}

func findNpx() (string, error) {
	path, err := exec.LookPath("npx")
	if err != nil {
		// Try common scoop location
		home, _ := os.UserHomeDir()
		scoopNpx := filepath.Join(home, "scoop", "apps", "nodejs", "current", "npx.cmd")
		if _, serr := os.Stat(scoopNpx); serr == nil {
			return scoopNpx, nil
		}
		return "", fmt.Errorf("npx not found on PATH: %w", err)
	}
	return path, nil
}

func runDaemon() error {
	npx, err := findNpx()
	if err != nil {
		return err
	}

	lf, err := openLog()
	if err != nil {
		return err
	}
	defer lf.Close()

	// Log to both file and stderr
	multi := io.MultiWriter(lf, os.Stderr)
	logger := log.New(multi, "[copilot-daemon] ", log.LstdFlags)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Printf("received %v, shutting down", sig)
		cancel()
	}()

	logger.Printf("copilot-daemon %s starting (npx=%s)", version, npx)

	backoff := initialBackoff
	for {
		logger.Println("starting copilot-api...")

		cmd := exec.CommandContext(ctx, npx, "copilot-api@latest", "start")
		cmd.Stdout = multi
		cmd.Stderr = multi
		// Inherit environment so copilot-api can find GitHub auth
		cmd.Env = os.Environ()

		startTime := time.Now()
		err := cmd.Run()

		if ctx.Err() != nil {
			logger.Println("shutdown complete")
			return nil
		}

		elapsed := time.Since(startTime)
		logger.Printf("copilot-api exited after %v: %v", elapsed.Round(time.Second), err)

		// Reset backoff if process ran for a while (was healthy)
		if elapsed > 2*time.Minute {
			backoff = initialBackoff
		}

		logger.Printf("restarting in %v...", backoff)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			logger.Println("shutdown during backoff")
			return nil
		}

		// Increase backoff for next failure
		backoff *= time.Duration(backoffFactor)
		if backoff > maxBackoff {
			backoff = maxBackoff
		}

		// Rotate log between restarts
		lf.Sync()
		rotateLog(logFile())
	}
}
