package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// Create a VBS launcher next to the exe.
	// WshShell.Run with window style 0 = completely hidden.
	// The "True" arg makes VBS wait for the process, so Task Scheduler can track it.
	vbsPath := filepath.Join(filepath.Dir(exe), "copilot-daemon-launcher.vbs")
	vbsContent := fmt.Sprintf(
		"Set WshShell = CreateObject(\"WScript.Shell\")\r\n"+
			"WshShell.Run \"\"\"%s\"\" start --foreground\", 0, True\r\n",
		exe,
	)
	if err := os.WriteFile(vbsPath, []byte(vbsContent), 0644); err != nil {
		return fmt.Errorf("write VBS launcher: %w", err)
	}

	// Remove existing task first (ignore errors)
	runSchtasks("/delete", "/tn", taskName, "/f")

	xml := buildTaskXML(vbsPath)

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
	fmt.Printf("  Logs:   copilot-daemon logs\n")
	fmt.Printf("  Status: copilot-daemon status\n")
	fmt.Printf("  Port:   http://localhost:4141\n")
	return nil
}

func uninstall() error {
	// Stop via IPC first (graceful)
	if conn, err := dialDaemon(); err == nil {
		fmt.Fprintln(conn, "STOP")
		conn.Close()
	}

	// Then remove the scheduled task
	runSchtasks("/end", "/tn", taskName)

	out, err := runSchtasks("/delete", "/tn", taskName, "/f")
	if err != nil {
		if strings.Contains(string(out), "does not exist") {
			fmt.Println("Task not found — nothing to remove.")
			return nil
		}
		return fmt.Errorf("remove task: %w\n%s", err, out)
	}

	// Clean up VBS launcher
	exe, _ := selfExe()
	if exe != "" {
		os.Remove(filepath.Join(filepath.Dir(exe), "copilot-daemon-launcher.vbs"))
	}

	fmt.Println("Scheduled task removed:", taskName)
	return nil
}

func status() error {
	return queryStatus()
}

func runSchtasks(args ...string) (string, error) {
	cmd := exec.Command("schtasks.exe", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func buildTaskXML(launcherPath string) string {
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
      <Command>wscript.exe</Command>
      <Arguments>"%s"</Arguments>
    </Exec>
  </Actions>
</Task>`, user, user, launcherPath)
}
