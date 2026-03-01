# localfreshllm

CLI and server for talking to local Ollama models and Anthropic Claude from the terminal. Streaming output, session history, pipe support, markdown rendering, and multi-device server mode with tool support.

## Quick Start

```bash
git clone https://github.com/dev-ben-c/localfreshllm.git
cd localfreshllm
./deploy.sh
```

The deploy script provides three options:

1. **Client** — build, install, and connect to an existing server
2. **Server** — full deploy with systemd, UFW, and key generation
3. **Advanced** — standalone install, Playwright setup, manual key entry

### Prerequisites

- **Go 1.23+**
- **Ollama** running locally (`ollama serve`)
- `../localfreshsearch` directory (sibling repo, required by `go.mod` replace directive)
- **Server mode** additionally requires `systemctl`, `ufw`, and `sudo`

### Manual Install

```bash
go build -o localfreshllm .
sudo cp localfreshllm /usr/local/bin/
```

## Usage

```bash
# One-shot query
localfreshllm "what is the meaning of life"

# Use a specific model
localfreshllm -m qwen2.5:7b "explain goroutines"

# Pipe mode — stdin as context
cat main.go | localfreshllm -m claude-sonnet-4-6 "review this code"

# Interactive REPL
localfreshllm -m qwen3:32b

# List all available models (Ollama + Claude)
localfreshllm --list

# Render markdown output with glamour
localfreshllm --render "write a fibonacci function in Go"

# Use a system prompt
localfreshllm -s "respond only in haiku" "tell me about rust"

# Use a built-in persona preset
localfreshllm -p coder "write a linked list in Go"
localfreshllm -p shell "find large files over 1GB"

# View saved conversations
localfreshllm --history

# Resume a previous conversation (prefix match)
localfreshllm --resume abc1
```

## Server Mode

Run as an API server with device authentication, per-device sessions, and SSE streaming.

```bash
# Start the server (deploy.sh handles this via systemd)
localfreshllm serve --addr 0.0.0.0:8400

# Register a device
curl -X POST http://<server>:8400/v1/devices/register \
  -H 'Content-Type: application/json' \
  -d '{"name":"my-laptop","registration_key":"<master-key>"}'

# Use as a client (after registering)
export LOCALFRESH_SERVER="http://<server>:8400"
export LOCALFRESH_KEY="<device-token>"
localfreshllm "hello from the client"
```

### API Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/health` | No | Server health check |
| POST | `/v1/devices/register` | No | Register a device (requires master key) |
| POST | `/v1/chat` | Bearer | Stream a chat response (SSE) |
| GET | `/v1/models` | Bearer | List available models |
| GET | `/v1/devices/me` | Bearer | Get device profile |
| PUT | `/v1/devices/me` | Bearer | Update device profile |
| GET | `/v1/sessions` | Bearer | List sessions |
| GET | `/v1/sessions/{id}` | Bearer | Get session (prefix match) |
| DELETE | `/v1/sessions/{id}` | Bearer | Delete session |

## Flags

| Flag | Short | Description |
|---|---|---|
| `--model` | `-m` | Model name (default: `claude-opus-4-6`) |
| `--system` | `-s` | Custom system prompt |
| `--persona` | `-p` | Named preset: `coder`, `reviewer`, `writer`, `shell` |
| `--server` | | Server URL for client mode |
| `--key` | | Device bearer token for client mode |
| `--list` | | List available models from Ollama and Anthropic |
| `--history` | | List saved conversations |
| `--resume` | | Resume a session by ID or prefix |
| `--render` | | Buffer output and render markdown with glamour |

## REPL Commands

| Command | Description |
|---|---|
| `/model <name>` | Switch model mid-conversation |
| `/clear` | Clear conversation history |
| `/history` | List saved sessions |
| `/location <city>` | Set location for weather tools |
| `/quit` | Exit |

## Session Storage

Conversations are saved as JSON in `~/.local/share/localfreshllm/history/` (respects `XDG_DATA_HOME`). In server mode, sessions are scoped per device under `~/.local/share/localfreshllm/devices/<id>/history/`.

## Environment Variables

| Variable | Purpose |
|---|---|
| `ANTHROPIC_API_KEY` | API key for Claude models |
| `OLLAMA_HOST` | Ollama server address (default: `http://127.0.0.1:11434`) |
| `XDG_DATA_HOME` | Base directory for session storage |
| `LOCALFRESH_MASTER_KEY` | Master key for server device registration |
| `LOCALFRESH_SERVER` | Server URL for client mode |
| `LOCALFRESH_KEY` | Device bearer token for client mode |
