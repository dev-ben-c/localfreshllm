# localfreshllm

CLI for talking to local Ollama models and Anthropic Claude from the terminal. Streaming output, session history, pipe support, and markdown rendering.

## Install

```bash
git clone https://github.com/dev-ben-c/localfreshllm.git
cd localfreshllm
go build -o localfreshllm .
```

Move the binary somewhere on your PATH:

```bash
sudo mv localfreshllm /usr/local/bin/
```

## Prerequisites

- **Go 1.22+** to build
- **Ollama** running locally for local models (`ollama serve`)
- **`ANTHROPIC_API_KEY`** env var set for Claude models

## Usage

```bash
# One-shot query (default model: claude-opus-4-6)
localfreshllm "what is the meaning of life"

# Use a local Ollama model
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

## Flags

| Flag | Short | Description |
|---|---|---|
| `--model` | `-m` | Model name (default: `claude-opus-4-6`) |
| `--system` | `-s` | Custom system prompt |
| `--persona` | `-p` | Named preset: `coder`, `reviewer`, `writer`, `shell` |
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
| `/quit` | Exit |

## Session Storage

Conversations are saved as JSON in `~/.local/share/localfreshllm/history/` (respects `XDG_DATA_HOME`). Sessions auto-save after each response in one-shot and REPL modes. Pipe mode does not save history.

## Environment Variables

| Variable | Purpose |
|---|---|
| `ANTHROPIC_API_KEY` | API key for Claude models |
| `OLLAMA_HOST` | Ollama server address (default: `http://127.0.0.1:11434`) |
| `XDG_DATA_HOME` | Base directory for session storage |
