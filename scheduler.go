package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

const taskName = "CopilotAPIDaemon"

func selfExe() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine own path: %w", err)
	}
	return exe, nil
}

func install() error {
	exe, err := selfExe()
	if err != nil {
		return err
	}

	// Remove existing task first (ignore errors)
	runSchtasks("/delete", "/tn", taskName, "/f")

	// XML task definition gives us full control over restart settings,
	// which schtasks /create flags don't support.
	xml := buildTaskXML(exe)

	// Write XML to temp file
	tmpFile, err := os.CreateTemp("", "copilot-daemon-task-*.xml")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(xml); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write task XML: %w", err)
	}
	tmpFile.Close()

	// Import the task
	out, err := runSchtasks("/create", "/tn", taskName, "/xml", tmpFile.Name())
	if err != nil {
		return fmt.Errorf("register task: %w\n%s", err, out)
	}

	fmt.Println("Scheduled task registered:", taskName)

	// Start immediately
	out, err = runSchtasks("/run", "/tn", taskName)
	if err != nil {
		return fmt.Errorf("start task: %w\n%s", err, out)
	}

	fmt.Println("Daemon started.")
	fmt.Printf("  Logs: %s\n", logFile())
	fmt.Println("  Port: http://localhost:4141")
	return nil
}

func uninstall() error {
	// Stop first
	runSchtasks("/end", "/tn", taskName)

	out, err := runSchtasks("/delete", "/tn", taskName, "/f")
	if err != nil {
		if strings.Contains(string(out), "does not exist") {
			fmt.Println("Task not found — nothing to remove.")
			return nil
		}
		return fmt.Errorf("remove task: %w\n%s", err, out)
	}

	fmt.Println("Scheduled task removed:", taskName)
	return nil
}

func status() error {
	// Check scheduled task
	out, err := runSchtasks("/query", "/tn", taskName, "/fo", "LIST")
	if err != nil {
		fmt.Println("Scheduled task: not registered")
	} else {
		taskState := "unknown"
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "Status:") {
				taskState = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "Status:"))
				break
			}
		}
		fmt.Println("Scheduled task:", taskState)
	}

	// Check port 4141
	conn, err := net.DialTimeout("tcp", "localhost:4141", 2*time.Second)
	if err != nil {
		fmt.Println("copilot-api:    not reachable on :4141")
	} else {
		conn.Close()
		fmt.Println("copilot-api:    listening on :4141")
	}

	return nil
}

func runSchtasks(args ...string) (string, error) {
	cmd := exec.Command("schtasks.exe", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func buildTaskXML(exePath string) string {
	user := os.Getenv("USERDOMAIN") + `\` + os.Getenv("USERNAME")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>copilot-api process supervisor — keeps the Copilot API proxy running on port 4141</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>%s</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>%s</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>999</Count>
    </RestartOnFailure>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>start</Arguments>
    </Exec>
  </Actions>
</Task>`, user, user, exePath)
}
