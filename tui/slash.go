package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/rabidclock/localfreshllm/backend"
	"github.com/rabidclock/localfreshllm/client"
	"github.com/rabidclock/localfreshllm/render"
	"github.com/rabidclock/localfreshllm/service"

	"github.com/dev-ben-c/localfreshsearch/weather"
)

type slashResult struct {
	quit      bool
	info      string
	modelPick bool
	ttsToggle bool

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
		return slashResult{info: "Voice toggle is handled via Ctrl+Space or F5 (requires client mode with --server)"}

	case "/tts":
		return slashResult{ttsToggle: true}

	case "/timer", "/timers":
		return handleTimer(parts[1:])

	case "/help":
		return slashResult{info: helpText()}
	}

	return slashResult{info: fmt.Sprintf("Unknown command: %s (try /help)", cmd)}
}

func handleModelSwitch(arg string, cfg *Config) slashResult {
	models := service.ListModels(context.Background())

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
	models := service.ListModels(context.Background())
	if len(models) == 0 {
		return slashResult{info: "No models available."}
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
	models := service.ListModels(context.Background())
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
		"  /voice         - voice input info (Ctrl+Space / F5)",
		"  /tts           - toggle text-to-speech",
		"  /timer <dur>   - start a timer (e.g. 5m, 1h30m)",
		"  /timer cancel  - cancel a timer by number",
		"  /timers        - list active timers",
		"  /quit          - exit",
		"",
		"Navigation:",
		"  PgUp/PgDn      - scroll chat",
		"  Up/Down         - input history",
	}, "\n")
}

