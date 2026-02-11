# copilot-daemon

Process supervisor that keeps [copilot-api](https://github.com/ericc-ch/copilot-api) running as a background daemon on Windows. Provides an OpenAI-compatible API proxy for GitHub Copilot on `localhost:4141`.

## Install

```powershell
scoop bucket add copilot-daemon https://github.com/jack-work/scoop-copilot-daemon
scoop install copilot-daemon
```

### Prerequisites

- [Node.js](https://nodejs.org/) (for `npx`)
- GitHub Copilot subscription
- Authenticated: `npx copilot-api@latest auth`

## Usage

```powershell
# Register as scheduled task + start immediately
copilot-daemon install

# Check if running
copilot-daemon status

# Run in foreground (for debugging)
copilot-daemon start

# Remove scheduled task
copilot-daemon uninstall
```

## What it does

- Spawns `npx copilot-api@latest start` as a supervised child process
- Restarts automatically on crash with exponential backoff (2s → 60s cap)
- Resets backoff after 2 minutes of healthy runtime
- Logs to `<exe-dir>/logs/copilot-daemon.log` with 10MB rotation
- `install` registers a Windows Task Scheduler task that starts on logon

## Build from source

```powershell
go build -o copilot-daemon.exe .
```
