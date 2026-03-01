# localfreshllm

A terminal-native LLM chat application with a full-screen TUI, multi-model support, tool use, and voice I/O. Talk to local [Ollama](https://ollama.com/) models or [Anthropic Claude](https://www.anthropic.com/) from the command line — interactively, in one-shot mode, or piped from stdin.

The interactive TUI features an animated braille-art lemon mascot, scrollable chat history, real-time streaming, markdown rendering, and slash commands for model switching, tool toggling, and more. A built-in server mode enables multi-device access with per-device sessions, bearer auth, and SSE streaming.

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

- **Animated lemon mascot** — braille-art lemon with expressive face (idle, thinking, speaking states)
- **Scrollable chat viewport** — PgUp/PgDn to browse history
- **Text input with history** — Up/Down arrow to recall previous messages
- **Streaming responses** — tokens appear in real-time, markdown rendered on completion
- **Slash commands** — model switching, tool toggling, TTS, timers, and more
- **Text-to-speech** — toggle with `/tts`, responses are spoken aloud via Piper
- **Voice mode** — continuous listening with wake word detection

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
| `/tts` | Toggle text-to-speech |
| `/voice` | Toggle voice mode (wake word: "Cedric") |
| `/timer <dur> [name]` | Start a countdown timer |
| `/timer cancel <N>` | Cancel timer by number |
| `/timers` | List active timers |
| `/device` | List or select audio input device |
| `/quit` | Exit |

### Navigation

| Key | Action |
|---|---|
| PgUp / PgDn | Scroll chat viewport |
| Up / Down | Recall input history |
| Ctrl+C | Quit |
| Ctrl+Space / F5 | Toggle voice mode (when mic enabled) |

### Voice Mode

Voice mode enables continuous listening with automatic speech detection. Toggle it with `/voice` or Ctrl+Space / F5. When active, the mascot's persona "Cedric" serves as the wake word — say "Cedric" to activate, then speak naturally. Detected speech is automatically transcribed and sent as a message.

### Natural Language Timers

Set timers using slash commands or natural language in chat:

- `/timer 5m` — 5 minute timer
- `/timer 30s eggs` — 30 second timer named "eggs"
- "Set a timer for 5 minutes"
- "Set a 30 second timer called eggs"
- "Timer for half an hour"

### TTS Text Processing

When TTS is enabled, responses are processed before speaking:

- Code blocks and URLs are removed for cleaner speech
- Common abbreviations are expanded (e.g., "Dr." → "Doctor")
- Sentence boundaries insert natural pauses

## Tools

When tools are enabled (default), the LLM can use:

- **Web search** — search the web via [SearXNG](https://github.com/searxng/searxng) and return titles, URLs, and snippets
- **Page reader** — fetch and extract text content from any URL via [Playwright](https://playwright.dev/)
- **Weather** — current conditions and forecast from [wttr.in](https://wttr.in/)
- **Date/time** — current local date, time, and timezone

Tools work with both Ollama and Claude models. Models that don't support tool calling automatically fall back to plain chat.

## Server Mode

Run as an API server with device authentication, per-device sessions, and SSE streaming.

```bash
# Start the server (deploy.sh handles this via systemd)
localfreshllm serve --addr 0.0.0.0:8400

# With audio services enabled
localfreshllm serve --addr 0.0.0.0:8400 \
  --whisper-url http://127.0.0.1:8081 \
  --piper-model /opt/piper/models/en_GB-semaine-medium.onnx \
  --piper-speaker 1

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

Voice input and output are split across client and server to keep the client lightweight:

- **Server**: [Whisper.cpp](https://github.com/ggerganov/whisper.cpp) (CUDA-accelerated STT) and [Piper](https://github.com/rhasspy/piper) (CPU TTS) run as sidecars
- **Client**: Mic capture via `parec`/`arecord` and playback via `paplay`/`aplay` — no CGO, just subprocess calls to standard Linux audio tools
- **Local TTS**: If Piper is installed locally (at `/opt/piper/`), TTS runs directly on the client without needing a server

### Piper TTS Setup

```bash
# Download and extract piper
sudo mkdir -p /opt/piper/models
curl -sL https://github.com/rhasspy/piper/releases/download/2023.11.14-2/piper_linux_x86_64.tar.gz | sudo tar xz -C /opt/piper --strip-components=1

# Download a multi-speaker voice model (see https://rhasspy.github.io/piper-samples/ for options)
sudo curl -sL -o /opt/piper/models/en_GB-semaine-medium.onnx \
  https://huggingface.co/rhasspy/piper-voices/resolve/v1.0.0/en/en_GB/semaine/medium/en_GB-semaine-medium.onnx
sudo curl -sL -o /opt/piper/models/en_GB-semaine-medium.onnx.json \
  https://huggingface.co/rhasspy/piper-voices/resolve/v1.0.0/en/en_GB/semaine/medium/en_GB-semaine-medium.onnx.json

# Test it (speaker 1 = male voice)
echo "Hello world" | LD_LIBRARY_PATH=/opt/piper /opt/piper/piper \
  --model /opt/piper/models/en_GB-semaine-medium.onnx --speaker 1 --output_file test.wav --quiet
```

The TUI auto-detects Piper at `/opt/piper/models/` on startup. Toggle TTS in the TUI with `/tts`.

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
| `--piper-speaker` | Piper speaker ID for multi-speaker models (e.g. `1`) |

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
│  Local TTS: piper subprocess (if installed)             │
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
- [Anthropic API](https://docs.anthropic.com/) — Claude model access
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — terminal UI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) — TUI components (viewport, text input)
- [Glamour](https://github.com/charmbracelet/glamour) — terminal markdown rendering
- [Lipgloss](https://github.com/charmbracelet/lipgloss) — terminal styling
- [Cobra](https://github.com/spf13/cobra) — CLI framework
- [Piper](https://github.com/rhasspy/piper) — fast local neural text-to-speech
- [Whisper.cpp](https://github.com/ggerganov/whisper.cpp) — speech-to-text inference
- [localfreshsearch](https://github.com/dev-ben-c/localfreshsearch) — web search, page reading, and weather tools
- [SearXNG](https://github.com/searxng/searxng) — metasearch engine for web search tools
- [wttr.in](https://github.com/chubin/wttr.in) — weather data API
- [Playwright for Go](https://github.com/playwright-community/playwright-go) — web page reading for search tools
- [go-isatty](https://github.com/mattn/go-isatty) — terminal detection
- [uuid](https://github.com/google/uuid) — session and device IDs
