# localfreshllm

CLI and server for talking to local Ollama models and Anthropic Claude from the terminal. Full-screen TUI with animated mascot, streaming output, session history, pipe support, markdown rendering, multi-device server mode with tool support, and voice I/O.

**Zero CGO, pure Go.** Single binary cross-compiles to `linux/arm64` for Raspberry Pi deployment.

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

- **Go 1.24+** (the deploy script will install it if missing)
- **Ollama** running locally (`ollama serve`)
- [localfreshsearch](https://github.com/dev-ben-c/localfreshsearch) sibling directory (auto-cloned by the deploy script)
- **Server mode** additionally requires `systemctl`, `ufw`, and `sudo`

### Manual Install

```bash
# Native build
CGO_ENABLED=0 go build -o localfreshllm .
sudo cp localfreshllm /usr/local/bin/

# Cross-compile for Raspberry Pi
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o localfreshllm-arm64 .
```

## Usage

```bash
# One-shot query
localfreshllm "what is the meaning of life"

# Use a specific model
localfreshllm -m qwen2.5:7b "explain goroutines"

# Pipe mode — stdin as context
cat main.go | localfreshllm -m claude-sonnet-4-6 "review this code"

# Interactive TUI
localfreshllm

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

## Interactive TUI

The interactive mode launches a full-screen Bubble Tea TUI with:

- **Animated lemon mascot** — idle, thinking (rotating dots), and speaking states
- **Scrollable chat viewport** — PgUp/PgDn to browse history
- **Text input with history** — Up/Down arrow to recall previous messages
- **Streaming responses** — tokens appear in real-time, markdown rendered on completion
- **Slash commands** — model switching, tool toggling, and more

```
┌─────────────────────────────────────────┐
│ 🍋 mascot (animated) │ model / status   │
├─────────────────────────────────────────┤
│ chat viewport (scrollable)              │
│   You: hello                            │
│   qwen3:14b:                            │
│   Here is my response...                │
├─────────────────────────────────────────┤
│ You: [text input___________________]    │
└─────────────────────────────────────────┘
```

### TUI Commands

| Command | Description |
|---|---|
| `/model` | Pick model from numbered list |
| `/model <name>` | Switch to named model |
| `/clear` | Clear conversation history |
| `/history` | Session history info |
| `/location <city>` | Set location for weather tools |
| `/tools` | Toggle web search tools |
| `/voice` | Voice input info |
| `/tts` | Text-to-speech info |
| `/quit` | Exit |

### Navigation

| Key | Action |
|---|---|
| PgUp / PgDn | Scroll chat viewport |
| Up / Down | Recall input history |
| Ctrl+C | Quit |
| Ctrl+Space / F5 | Push-to-talk toggle (when mic enabled) |

## Server Mode

Run as an API server with device authentication, per-device sessions, and SSE streaming.

```bash
# Start the server (deploy.sh handles this via systemd)
localfreshllm serve --addr 0.0.0.0:8400

# With audio services enabled
localfreshllm serve --addr 0.0.0.0:8400 \
  --whisper-url http://127.0.0.1:8081 \
  --piper-model /path/to/en_US-lessac-medium.onnx

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
| POST | `/v1/audio/transcribe` | Bearer | Speech-to-text (raw PCM → text) |
| POST | `/v1/audio/speak` | Bearer | Text-to-speech (text → WAV) |

Audio endpoints return 501 if `--whisper-url` / `--piper-model` are not set.

## Audio / Voice I/O

Voice input and output are split across client and server:

- **Server**: Whisper.cpp (CUDA-accelerated STT) and Piper (CPU TTS) run as sidecars
- **Client**: Mic capture via `parec`/`arecord` and playback via `paplay`/`aplay` — no CGO, just subprocess calls to standard Linux audio tools

This keeps the client binary lightweight enough for a Raspberry Pi while offloading GPU-heavy inference to the server.

## Flags

| Flag | Short | Description |
|---|---|---|
| `--model` | `-m` | Model name (default: `qwen3:14b`) |
| `--system` | `-s` | Custom system prompt |
| `--persona` | `-p` | Named preset: `coder`, `reviewer`, `writer`, `shell` |
| `--server` | | Server URL for client mode |
| `--key` | | Device bearer token for client mode |
| `--list` | | List available models from Ollama and Anthropic |
| `--history` | | List saved conversations |
| `--resume` | | Resume a session by ID or prefix |
| `--render` | | Buffer output and render markdown with glamour |
| `--tools` | | Enable web search and page reading tools (default: true) |

### Server Flags

| Flag | Description |
|---|---|
| `--addr` | Listen address (default: `0.0.0.0:8400`) |
| `--key` | Master registration key |
| `--whisper-url` | Whisper.cpp server URL for STT (e.g. `http://127.0.0.1:8081`) |
| `--piper-model` | Piper TTS model path for speech synthesis |

## Architecture

```
┌─ Client (Pi / tablet / desktop) ────────────────────────┐
│  Bubble Tea TUI (pure Go, no CGO)                       │
│  ┌──────────┐  ┌──────────┐  ┌────────────────────────┐ │
│  │ Mascot   │  │ Viewport │  │ Text Input / PTT key   │ │
│  │ (anim)   │  │ (chat)   │  │                        │ │
│  └──────────┘  └──────────┘  └────────────────────────┘ │
│  Audio: parec/arecord subprocess → raw PCM upload       │
│  Audio: WAV download → paplay/aplay subprocess          │
└─────────────────────┬───────────────────────────────────┘
                      │ SSE + REST
┌─ Server (GPU host) ─┴──────────────────────────────────┐
│  POST /v1/chat              (SSE streaming)             │
│  POST /v1/audio/transcribe  (audio → text)              │
│  POST /v1/audio/speak       (text → WAV)                │
│                                                         │
│  Whisper.cpp HTTP server (sidecar, CUDA)                │
│  Piper TTS (subprocess, CPU)                            │
│  Ollama (CUDA)                                          │
└─────────────────────────────────────────────────────────┘
```

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

## Acknowledgments

Built with these excellent open-source projects:

- [Ollama](https://ollama.com/) — local LLM inference
- [Cobra](https://github.com/spf13/cobra) — CLI framework
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — terminal UI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) — TUI components (viewport, text input)
- [Glamour](https://github.com/charmbracelet/glamour) — terminal markdown rendering
- [Lipgloss](https://github.com/charmbracelet/lipgloss) — terminal styling
- [Playwright for Go](https://github.com/playwright-community/playwright-go) — web scraping for search tools
- [go-isatty](https://github.com/mattn/go-isatty) — terminal detection
- [uuid](https://github.com/google/uuid) — session and device IDs
