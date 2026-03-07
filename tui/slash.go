package tui

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/dev-ben-c/localfreshllm/audio/capture"
	"github.com/dev-ben-c/localfreshllm/backend"
	"github.com/dev-ben-c/localfreshllm/client"
	"github.com/dev-ben-c/localfreshllm/render"
	"github.com/dev-ben-c/localfreshllm/service"

	"github.com/dev-ben-c/localfreshsearch/weather"
)

type slashResult struct {
	quit      bool
	info      string
	modelPick bool
	ttsToggle   bool
	voiceToggle bool

	// Timer actions.
	timerAdd    *Timer // Non-nil to create a new timer.
	timerCancel int    // 1-based index to cancel, 0 means no cancel.
	timerClear  bool   // Clear all timers.
}

type slashResultMsg = slashResult

func handleSlash(input string, cfg *Config) slashResult {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return slashResult{}
	}
	cmd := parts[0]

	switch cmd {
	case "/quit", "/exit", "/q":
		return slashResult{quit: true}

	case "/model":
		if len(parts) >= 2 {
			return handleModelSwitch(parts[1], cfg)
		}
		return showModelPicker(cfg)

	case "/clear":
		return handleClear(cfg)

	case "/history":
		return handleHistory()

	case "/tools":
		cfg.EnableTools = !cfg.EnableTools
		status := "enabled"
		if !cfg.EnableTools {
			status = "disabled"
		}
		return slashResult{info: "Tools " + status}

	case "/location":
		return handleLocation(parts[1:], cfg)

	case "/voice":
		return slashResult{voiceToggle: true}

	case "/device":
		return handleDevice(parts[1:], cfg)

	case "/tts":
		return slashResult{ttsToggle: true}

	case "/timer", "/timers":
		return handleTimer(parts[1:])

	case "/help":
		return slashResult{info: helpText()}
	}

	return slashResult{info: fmt.Sprintf("Unknown command: %s (try /help)", cmd)}
}

func listModels(cfg *Config) []string {
	ctx := context.Background()
	if cfg.IsClient {
		models, err := cfg.Backend.ListModels(ctx)
		if err != nil {
			log.Printf("Remote model listing failed: %v", err)
			return nil
		}
		return models
	}
	return service.ListModels(ctx)
}

func handleModelSwitch(arg string, cfg *Config) slashResult {
	models := listModels(cfg)

	// Try as number first.
	if n, err := strconv.Atoi(arg); err == nil && n >= 1 && n <= len(models) {
		return switchModel(models[n-1], cfg)
	}
	return switchModel(arg, cfg)
}

func switchModel(name string, cfg *Config) slashResult {
	newBackend := backend.ForModel(name)
	if err := newBackend.Validate(); err != nil {
		return slashResult{info: fmt.Sprintf("Error: %v", err)}
	}
	cfg.Model = name
	cfg.Backend = newBackend
	if cfg.Session != nil {
		cfg.Session.Model = name
	}
	if cfg.UserConfig != nil {
		cfg.UserConfig.Model = name
		cfg.UserConfig.Save()
	}
	return slashResult{info: fmt.Sprintf("Switched to %s (saved as default)", name)}
}

func showModelPicker(cfg *Config) slashResult {
	models := listModels(cfg)
	if len(models) == 0 {
		return slashResult{info: "No models available. Is Ollama running? (check: curl http://127.0.0.1:11434/api/tags)"}
	}

	var sb strings.Builder
	sb.WriteString("Select a model:\n")
	for i, m := range models {
		marker := "  "
		if m == cfg.Model {
			marker = "> "
		}
		sb.WriteString(fmt.Sprintf("%s[%d] %s\n", marker, i+1, m))
	}
	sb.WriteString("Enter number:")

	return slashResult{info: sb.String(), modelPick: true}
}

func handleModelPickInput(input string, cfg *Config) slashResult {
	models := listModels(cfg)
	n, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || n < 1 || n > len(models) {
		return slashResult{info: "Invalid selection."}
	}
	return switchModel(models[n-1], cfg)
}

func handleClear(cfg *Config) slashResult {
	if cfg.Session != nil {
		cfg.Session.Messages = nil
	}
	if cfg.IsClient {
		if remote, ok := cfg.Backend.(*client.RemoteBackend); ok {
			remote.ClearSession()
		}
	}
	return slashResult{info: "Conversation cleared."}
}

func handleHistory() slashResult {
	// Import from cmd package's runHistory would create a cycle.
	// Just show a hint to use the --history flag.
	return slashResult{info: "Use --history flag or view sessions in " + render.DimStyle.Render("~/.local/share/localfreshllm/history/")}
}

func handleLocation(args []string, cfg *Config) slashResult {
	if cfg.IsClient {
		if len(args) == 0 {
			return slashResult{info: "Usage: /location <city>"}
		}
		loc := strings.Join(args, " ")
		if remote, ok := cfg.Backend.(*client.RemoteBackend); ok {
			if err := remote.UpdateLocation(loc); err != nil {
				return slashResult{info: fmt.Sprintf("Failed to update location: %v", err)}
			}
			return slashResult{info: fmt.Sprintf("Location set to %s", loc)}
		}
	}

	if cfg.UserConfig == nil {
		return slashResult{info: "Config not available."}
	}

	if len(args) == 0 {
		if cfg.UserConfig.Location != "" {
			return slashResult{info: fmt.Sprintf("Current location: %s\nUsage: /location <city or zip>", cfg.UserConfig.Location)}
		}
		return slashResult{info: "No location set. Usage: /location <city or zip>"}
	}

	loc := strings.Join(args, " ")
	cfg.UserConfig.Location = loc
	if err := cfg.UserConfig.Save(); err != nil {
		return slashResult{info: fmt.Sprintf("Failed to save config: %v", err)}
	}
	weather.NewClient().Prefetch(loc)
	return slashResult{info: fmt.Sprintf("Location set to %s", loc)}
}

func handleDevice(args []string, cfg *Config) slashResult {
	if len(args) == 0 {
		// List available sources.
		sources, err := capture.ListSources()
		if err != nil {
			return slashResult{info: fmt.Sprintf("Error listing devices: %v", err)}
		}
		if len(sources) == 0 {
			return slashResult{info: "No audio input devices found."}
		}

		current := cfg.AudioDevice
		if current == "" {
			current = "(system default)"
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Audio input: %s\n\nAvailable devices:\n", current))
		for i, s := range sources {
			marker := "  "
			if s.Name == cfg.AudioDevice {
				marker = "> "
			}
			sb.WriteString(fmt.Sprintf("%s[%d] %s\n", marker, i+1, s.Description))
		}
		sb.WriteString("\nUsage: /device <number> or /device <name>\n/device default — use system default")
		return slashResult{info: sb.String()}
	}

	arg := strings.Join(args, " ")

	if arg == "default" || arg == "reset" {
		cfg.AudioDevice = ""
		return slashResult{info: "Audio input set to system default"}
	}

	// Try as number.
	if n, err := strconv.Atoi(arg); err == nil {
		sources, listErr := capture.ListSources()
		if listErr != nil {
			return slashResult{info: fmt.Sprintf("Error: %v", listErr)}
		}
		if n < 1 || n > len(sources) {
			return slashResult{info: fmt.Sprintf("Invalid device number (1-%d)", len(sources))}
		}
		cfg.AudioDevice = sources[n-1].Name
		return slashResult{info: fmt.Sprintf("Audio input set to %s", sources[n-1].Description)}
	}

	// Use as literal device name.
	cfg.AudioDevice = arg
	return slashResult{info: fmt.Sprintf("Audio input set to %s", arg)}
}

func handleTimer(args []string) slashResult {
	// /timer or /timers with no args → list (handled by model since it holds the timers).
	if len(args) == 0 {
		return slashResult{info: "_timer_list_"}
	}

	sub := args[0]

	switch sub {
	case "list":
		return slashResult{info: "_timer_list_"}

	case "cancel":
		if len(args) < 2 {
			return slashResult{info: "Usage: /timer cancel <number>"}
		}
		n, err := strconv.Atoi(args[1])
		if err != nil || n < 1 || n > maxTimers {
			return slashResult{info: fmt.Sprintf("Invalid timer number (1-%d)", maxTimers)}
		}
		return slashResult{timerCancel: n}

	case "clear":
		return slashResult{timerClear: true}

	default:
		// /timer <duration> [name...]
		d, err := parseDuration(sub)
		if err != nil {
			return slashResult{info: err.Error()}
		}
		if d <= 0 {
			return slashResult{info: "Duration must be positive"}
		}
		if d > maxDuration {
			return slashResult{info: fmt.Sprintf("Maximum timer duration is %s", maxDuration)}
		}
		name := "timer"
		if len(args) >= 2 {
			name = strings.Join(args[1:], " ")
		}
		return slashResult{timerAdd: &Timer{
			Name:     name,
			Duration: d,
		}}
	}
}

func helpText() string {
	return strings.Join([]string{
		"Commands:",
		"  /model         - pick model from list",
		"  /model <name>  - switch to named model",
		"  /location      - show or set default location",
		"  /clear         - clear conversation",
		"  /history       - session history info",
		"  /tools         - toggle web search tools",
		"  /voice         - toggle voice mode (wake word: \"cedric\")",
		"  /tts           - toggle text-to-speech",
		"  /device        - list or set audio input device",
		"  /timer <dur>   - start a timer (e.g. 5m, 1h30m)",
		"  /timer cancel  - cancel a timer by number",
		"  /timers        - list active timers",
		"  /quit          - exit",
		"",
		"Navigation:",
		"  PgUp/PgDn      - scroll chat",
		"  Up/Down         - input history",
		"  Ctrl+Space/F5   - toggle voice mode",
	}, "\n")
}

