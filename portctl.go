package main

import (
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// isPortInUse checks if the given port is already listening.
func isPortInUse(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// killPortHolder finds and kills the process listening on the given port.
// Returns nil if the port is now free (or was already free).
func killPortHolder(port int) error {
	pid, err := findPidOnPort(port)
	if err != nil || pid == "" {
		return nil // port is free or we can't find the holder
	}

	// Kill the process
	cmd := exec.Command("taskkill", "/F", "/PID", pid)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to kill PID %s: %w\n%s", pid, err, out)
	}

	// Wait briefly for port to be released
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if !isPortInUse(port) {
			return nil
		}
	}

	return fmt.Errorf("port %d still in use after killing PID %s", port, pid)
}

// findPidOnPort uses netstat to find the PID listening on a port.
func findPidOnPort(port int) (string, error) {
	cmd := exec.Command("netstat", "-ano", "-p", "TCP")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	target := fmt.Sprintf(":%d ", port)
	re := regexp.MustCompile(`\s+LISTENING\s+(\d+)\s*$`)

	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, target) {
			continue
		}
		if !strings.Contains(line, "LISTENING") {
			continue
		}
		matches := re.FindStringSubmatch(line)
		if len(matches) >= 2 {
			return matches[1], nil
		}
	}

	return "", nil
}
