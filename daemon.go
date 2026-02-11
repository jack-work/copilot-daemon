package main

import (
	"context"
	"fmt"
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

	createNoWindow  = 0x08000000
	detachedProcess = 0x00000008
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
		home, _ := os.UserHomeDir()
		scoopNpx := filepath.Join(home, "scoop", "apps", "nodejs", "current", "npx.cmd")
		if _, serr := os.Stat(scoopNpx); serr == nil {
			return scoopNpx, nil
		}
		return "", fmt.Errorf("npx not found on PATH: %w", err)
	}
	return path, nil
}

// daemonize re-launches this process detached with no console window, then exits.
func daemonize() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine own path: %w", err)
	}

	cmd := exec.Command(exe, "start", "--foreground")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | createNoWindow,
	}
	// Pass environment so child can find npx, GitHub auth, etc.
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	fmt.Printf("Daemon started (PID %d)\n", cmd.Process.Pid)
	fmt.Println("  Status:  copilot-daemon status")
	fmt.Println("  Logs:    copilot-daemon logs")
	fmt.Println("  Stop:    copilot-daemon stop")
	return nil
}

// broadcastWriter writes to a log file and broadcasts to IPC log subscribers.
type broadcastWriter struct {
	file *os.File
	ipc  *IPCServer
}

func (w *broadcastWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	if w.ipc != nil {
		w.ipc.BroadcastLog(p)
	}
	return n, err
}

func runDaemon() error {
	cfg := loadConfig()

	npx, err := findNpx()
	if err != nil {
		return err
	}

	lf, err := openLog()
	if err != nil {
		return err
	}
	defer lf.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start IPC server (named pipe)
	ipc, err := newIPCServer(cfg.Port, cancel)
	if err != nil {
		// Log but don't fail — daemon can still work without IPC
		log.New(lf, "[copilot-daemon] ", log.LstdFlags).Printf("WARNING: IPC pipe unavailable: %v", err)
	}

	// Writer that goes to file + IPC log subscribers
	bw := &broadcastWriter{file: lf, ipc: ipc}
	logger := log.New(bw, "[copilot-daemon] ", log.LstdFlags)

	if ipc != nil {
		ipc.logger = logger
		go ipc.Serve(ctx)
		defer ipc.Close()
	}

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Printf("received %v, shutting down", sig)
		cancel()
	}()

	logger.Printf("copilot-daemon %s starting (npx=%s, port=%d, pipe=%s)", version, npx, cfg.Port, pipeName)

	backoff := initialBackoff
	for {
		// Check if port is already in use
		if isPortInUse(cfg.Port) {
			if cfg.DoNotKillExisting {
				logger.Printf("port %d already in use and do_not_kill_existing=true, exiting", cfg.Port)
				return nil
			}
			logger.Printf("port %d in use, killing existing process...", cfg.Port)
			if err := killPortHolder(cfg.Port); err != nil {
				logger.Printf("failed to free port: %v", err)
			} else {
				logger.Printf("port %d freed", cfg.Port)
			}
		}

		logger.Println("starting copilot-api...")

		cmd := exec.Command(npx, "copilot-api@latest", "start")
		cmd.Stdout = bw
		cmd.Stderr = bw
		cmd.Env = os.Environ()
		// Child process also gets no window
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: createNoWindow,
		}

		startTime := time.Now()

		if startErr := cmd.Start(); startErr != nil {
			logger.Printf("failed to start copilot-api: %v", startErr)
		} else {
			// Wait for either the process to exit or context cancellation (stop).
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()

			select {
			case err = <-done:
				// Process exited on its own
			case <-ctx.Done():
				// Shutdown requested — kill entire process tree
				logger.Println("killing copilot-api process tree...")
				killTree := exec.Command("taskkill", "/t", "/f", "/pid", fmt.Sprintf("%d", cmd.Process.Pid))
				killTree.Run()
				<-done
				logger.Println("shutdown complete")
				return nil
			}

			if ctx.Err() != nil {
				logger.Println("shutdown complete")
				return nil
			}

			elapsed := time.Since(startTime)
			logger.Printf("copilot-api exited after %v: %v", elapsed.Round(time.Second), err)

			if elapsed > 2*time.Minute {
				backoff = initialBackoff
			}
		}

		logger.Printf("restarting in %v...", backoff)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			logger.Println("shutdown during backoff")
			return nil
		}

		backoff *= time.Duration(backoffFactor)
		if backoff > maxBackoff {
			backoff = maxBackoff
		}

		lf.Sync()
		rotateLog(logFile())
	}
}
