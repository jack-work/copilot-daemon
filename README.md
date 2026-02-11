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
- On startup, if the port is already in use, **kills the existing holder** and takes over
- Restarts automatically on crash with exponential backoff (2s -> 60s cap)
- Resets backoff after 2 minutes of healthy runtime
- Runs headless — logs to `<exe-dir>/logs/copilot-daemon.log` with 10MB rotation
- When run in a terminal (`copilot-daemon start`), also prints to stderr
- `install` registers a Windows Task Scheduler task that starts on logon

## Configuration

Place a `config.json` next to the executable (optional):

```json
{
  "port": 4141,
  "do_not_kill_existing": false
}
```

| Key | Default | Description |
|-----|---------|-------------|
| `port` | `4141` | Port copilot-api listens on |
| `do_not_kill_existing` | `false` | If `true`, exit instead of killing an existing process on the port |

## Build from source

```powershell
go build -ldflags "-s -w" -o copilot-daemon.exe .
```
