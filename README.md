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
# Register as startup task + start immediately
copilot-daemon install

# Start in background (no window, detaches immediately)
copilot-daemon start

# Query daemon status via named pipe
copilot-daemon status

# Tail live log stream via named pipe
copilot-daemon logs

# Gracefully stop (kills entire process tree)
copilot-daemon stop

# Remove scheduled task
copilot-daemon uninstall
```

## How it works

- Spawns `npx copilot-api@latest start` as a supervised child process
- **No visible window** — `start` detaches with `DETACHED_PROCESS | CREATE_NO_WINDOW`; Task Scheduler uses a VBS launcher with hidden window style
- **Named pipe IPC** (`\\.\pipe\copilot-daemon`) for `status`, `stop`, and `logs` commands
- `logs` streams live output via the named pipe — connect from any terminal
- On port conflict, kills the existing holder and takes over (configurable)
- Auto-restarts on crash with exponential backoff (2s -> 60s cap, resets after 2min healthy)
- Logs to `<exe-dir>/logs/copilot-daemon.log` with 10MB rotation
- `stop` uses `taskkill /t /f` to kill the entire npx/node process tree

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
