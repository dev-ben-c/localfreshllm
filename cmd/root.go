package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chzyer/readline"
	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/rabidclock/localfreshllm/backend"
	"github.com/rabidclock/localfreshllm/client"
	"github.com/rabidclock/localfreshllm/render"
	"github.com/rabidclock/localfreshllm/service"
	"github.com/rabidclock/localfreshllm/session"
	"github.com/rabidclock/localfreshllm/systemprompt"

	"github.com/dev-ben-c/localfreshsearch/weather"
)

var (
	flagModel   string
	flagSystem  string
	flagPersona string
	flagList    bool
	flagHistory bool
	flagResume  string
	flagRender  bool
	flagTools   bool
	flagServer  string
	flagKey     string

	cfg         *session.Config
	chatService *service.ChatService
)

var rootCmd = &cobra.Command{
	Use:   "localfreshllm [question]",
	Short: "CLI for local and cloud LLMs",
	Long:  "Talk to Ollama models and Claude from the terminal. Streaming, sessions, and pipe support.",
	// Suppress cobra's default error/usage on RunE errors.
	SilenceErrors: true,
	SilenceUsage:  true,
	Args:          cobra.ArbitraryArgs,
	RunE:          run,
}

func init() {
	rootCmd.Flags().StringVarP(&flagModel, "model", "m", "qwen3:14b", "Model name")
	rootCmd.Flags().StringVarP(&flagSystem, "system", "s", "", "Custom system prompt")
	rootCmd.Flags().StringVarP(&flagPersona, "persona", "p", "", "Named preset (coder, reviewer, writer, shell)")
	rootCmd.Flags().BoolVar(&flagList, "list", false, "List available models")
	rootCmd.Flags().BoolVar(&flagHistory, "history", false, "List saved conversations")
	rootCmd.Flags().StringVar(&flagResume, "resume", "", "Session ID (or prefix) to continue")
	rootCmd.Flags().BoolVar(&flagRender, "render", false, "Render markdown output with glamour")
	rootCmd.Flags().BoolVar(&flagTools, "tools", true, "Enable web search and page reading tools")
	rootCmd.Flags().StringVar(&flagServer, "server", "", "Server URL for client mode (or LOCALFRESH_SERVER env)")
	rootCmd.Flags().StringVar(&flagKey, "key", "", "API key for server auth (or LOCALFRESH_KEY env)")
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// serverURL returns the configured server URL from flag or env.
func serverURL() string {
	if flagServer != "" {
		return flagServer
	}
	return os.Getenv("LOCALFRESH_SERVER")
}

// serverKey returns the configured API key from flag or env.
func serverKey() string {
	if flagKey != "" {
		return flagKey
	}
	return os.Getenv("LOCALFRESH_KEY")
}

// historyFile returns the path to the readline history file.
func historyFile() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		dir = os.ExpandEnv("$HOME/.local/share")
	}
	return dir + "/localfreshllm/.readline_history"
}

func run(cmd *cobra.Command, args []string) error {
	// Subcommand modes.
	if flagList {
		return runList()
	}
	if flagHistory {
		return runHistory()
	}

	cfg = session.LoadConfig()
	chatService = service.New()

	// Client mode: connect to remote server.
	if srvURL := serverURL(); srvURL != "" {
		return runClientMode(cmd, args, srvURL)
	}

	// Use saved default model unless --model was explicitly provided.
	if !cmd.Flags().Changed("model") && cfg.Model != "" {
		flagModel = cfg.Model
	}

	sysPrompt := systemprompt.Get(flagSystem, flagPersona)
	b := backend.ForModel(flagModel)
	if err := b.Validate(); err != nil {
		// Default Claude model unavailable — fall back to first Ollama model.
		ollama := backend.NewOllama()
		models, listErr := ollama.ListModels(context.Background())
		if listErr != nil || len(models) == 0 {
			return err
		}
		flagModel = models[0]
		b = ollama
		render.Infof("No API key — using %s", flagModel)
	}
	store := session.NewStore()

	// Load or create session.
	var sess *session.Session
	if flagResume != "" {
		var err error
		sess, err = store.FindByPrefix(flagResume)
		if err != nil {
			return err
		}
		flagModel = sess.Model
		b = backend.ForModel(flagModel)
		render.Infof("Resumed session %s (%s, %d messages)", sess.ID, sess.Model, len(sess.Messages))
	}

	stdinIsTTY := isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())

	// Pipe mode: stdin has piped data (not a TTY).
	if !stdinIsTTY {
		return runPipe(b, sysPrompt, args)
	}

	// One-shot mode: positional args provided.
	if len(args) > 0 {
		return runOneShot(b, sysPrompt, store, sess, args)
	}

	// REPL mode: interactive.
	return runREPL(b, sysPrompt, store, sess)
}

// runClientMode handles CLI operation when connected to a remote server.
func runClientMode(cmd *cobra.Command, args []string, srvURL string) error {
	key := serverKey()
	if key == "" {
		return fmt.Errorf("server mode requires --key or LOCALFRESH_KEY")
	}

	remote := client.New(srvURL, key)
	if err := remote.Validate(); err != nil {
		return fmt.Errorf("server connection failed: %w", err)
	}

	// Use saved default model unless --model was explicitly provided.
	if !cmd.Flags().Changed("model") && cfg.Model != "" {
		flagModel = cfg.Model
	}

	sysPrompt := systemprompt.Get(flagSystem, flagPersona)

	stdinIsTTY := isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())

	// Pipe mode.
	if !stdinIsTTY {
		return runPipe(remote, sysPrompt, args)
	}

	// One-shot mode — no local session save in client mode.
	if len(args) > 0 {
		return runClientOneShot(remote, sysPrompt, args)
	}

	// REPL mode — no local session save in client mode.
	return runClientREPL(remote, sysPrompt)
}

// runClientOneShot handles one-shot mode in client mode (no local session persistence).
func runClientOneShot(b backend.Backend, sysPrompt string, args []string) error {
	question := strings.Join(args, " ")
	messages := []backend.Message{{Role: "user", Content: question}}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT)
	defer cancel()

	fmt.Fprintln(os.Stderr, render.AssistantStyle.Render(flagModel+":"))

	req := service.ChatRequest{
		Model:        flagModel,
		Messages:     messages,
		SystemPrompt: sysPrompt,
		Location:     cfg.Location,
		EnableTools:  flagTools,
	}

	response, _, err := chatService.Chat(ctx, b, req, cliEmit)
	if !flagRender {
		fmt.Println()
	}

	if response != "" && flagRender {
		fmt.Print(render.RenderMarkdown(response))
	}

	return err
}

// runClientREPL handles REPL mode in client mode (no local session persistence).
func runClientREPL(b backend.Backend, sysPrompt string) error {
	var messages []backend.Message

	render.PrintLemonColored(render.LemonIdle)
	render.Infof("localfreshllm (remote) — model: %s — /help for commands, /quit to exit", flagModel)
	fmt.Println()

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          render.UserStyle.Render("You: "),
		HistoryFile:     historyFile(),
		InterruptPrompt: "^C",
		EOFPrompt:       "/quit",
	})
	if err != nil {
		return fmt.Errorf("readline init: %w", err)
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			break
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if strings.HasPrefix(input, "/") {
			parts := strings.Fields(input)
			switch parts[0] {
			case "/quit", "/exit", "/q":
				return nil
			case "/clear":
				messages = nil
				if remote, ok := b.(*client.RemoteBackend); ok {
					remote.ClearSession()
				}
				render.Infof("Conversation cleared.")
				continue
			case "/tools":
				flagTools = !flagTools
				if flagTools {
					render.Infof("Tools enabled")
				} else {
					render.Infof("Tools disabled")
				}
				continue
			case "/location":
				if len(parts) < 2 {
					render.Infof("Usage: /location <city>")
					continue
				}
				loc := strings.Join(parts[1:], " ")
				if remote, ok := b.(*client.RemoteBackend); ok {
					if err := remote.UpdateLocation(loc); err != nil {
						render.Infof("Failed to update location: %v", err)
					} else {
						render.Infof("Location set to %s", loc)
					}
				}
				continue
			case "/help":
				fmt.Println(render.SystemStyle.Render("Commands:"))
				fmt.Println(render.SystemStyle.Render("  /clear         — clear conversation"))
				fmt.Println(render.SystemStyle.Render("  /location      — set location for weather tools"))
				fmt.Println(render.SystemStyle.Render("  /tools         — toggle web search tools"))
				fmt.Println(render.SystemStyle.Render("  /quit          — exit"))
				continue
			default:
				render.Infof("Unknown command: %s (try /help)", parts[0])
				continue
			}
		}

		messages = append(messages, backend.Message{Role: "user", Content: input})

		fmt.Println(render.AssistantStyle.Render(flagModel + ":"))

		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT)

		req := service.ChatRequest{
			Model:        flagModel,
			Messages:     messages,
			SystemPrompt: sysPrompt,
			Location:     cfg.Location,
			EnableTools:  flagTools,
		}

		response, newMsgs, err := chatService.Chat(ctx, b, req, cliEmit)
		cancel()
		if !flagRender {
			fmt.Println()
		}
		fmt.Println()

		if response != "" || len(newMsgs) > 0 {
			for _, m := range newMsgs {
				messages = append(messages, m)
			}
			if response != "" {
				if flagRender {
					fmt.Print(render.RenderMarkdown(response))
					fmt.Println()
				}
				messages = append(messages, backend.Message{Role: "assistant", Content: response})
			}
		}

		if err != nil && ctx.Err() == nil {
			fmt.Println(render.ErrorStyle.Render(fmt.Sprintf("Error: %v", err)))
		}
	}

	return nil
}

// cliEmit handles ChatEvents for CLI output.
func cliEmit(ev service.ChatEvent) {
	switch ev.Type {
	case "token":
		if !flagRender {
			fmt.Print(ev.Token)
		}
	case "tool_call":
		fmt.Fprintf(os.Stderr, "%s\n", render.DimStyle.Render(service.FormatToolCallInfo(ev.ToolName)))
	}
}

// chatWithTools runs a Chat call using the service layer.
func chatWithTools(ctx context.Context, b backend.Backend, model string, messages []backend.Message, sysPrompt string, onToken backend.StreamCallback) (string, []backend.Message, error) {
	location := ""
	if cfg != nil {
		location = cfg.Location
	}

	req := service.ChatRequest{
		Model:        model,
		Messages:     messages,
		SystemPrompt: sysPrompt,
		Location:     location,
		EnableTools:  flagTools,
	}

	emit := func(ev service.ChatEvent) {
		switch ev.Type {
		case "token":
			if onToken != nil {
				onToken(ev.Token)
			}
		case "tool_call":
			fmt.Fprintf(os.Stderr, "%s\n", render.DimStyle.Render(service.FormatToolCallInfo(ev.ToolName)))
		}
	}

	return chatService.Chat(ctx, b, req, emit)
}

func runPipe(b backend.Backend, sysPrompt string, args []string) error {
	// Read stdin with a timeout to handle empty pipes that stay open
	// (e.g., when launched by process managers that don't close stdin).
	stdinCh := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go func() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			errCh <- err
			return
		}
		stdinCh <- data
	}()

	var stdinData []byte
	if len(args) > 0 {
		// If we have args, try to read stdin briefly but don't block forever.
		select {
		case data := <-stdinCh:
			stdinData = data
		case err := <-errCh:
			return fmt.Errorf("read stdin: %w", err)
		case <-time.After(100 * time.Millisecond):
			// No stdin data available, just use args as question.
		}
	} else {
		// No args: stdin IS the question, wait for it but with a reasonable timeout.
		select {
		case data := <-stdinCh:
			stdinData = data
		case err := <-errCh:
			return fmt.Errorf("read stdin: %w", err)
		case <-time.After(100 * time.Millisecond):
			return fmt.Errorf("no input provided (pipe stdin or pass a question as arguments)")
		}
	}

	question := strings.TrimSpace(string(stdinData))
	if len(args) > 0 {
		argQuestion := strings.TrimSpace(strings.Join(args, " "))
		if question != "" {
			// Args are the question, stdin is context.
			question = argQuestion + "\n\n" + question
		} else {
			question = argQuestion
		}
	}

	if question == "" {
		return fmt.Errorf("no input provided")
	}

	messages := []backend.Message{{Role: "user", Content: question}}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT)
	defer cancel()

	response, _, chatErr := chatWithTools(ctx, b, flagModel, messages, sysPrompt, func(token string) {
		fmt.Print(token)
	})
	_ = response
	fmt.Println()
	return chatErr
}

func runOneShot(b backend.Backend, sysPrompt string, store *session.Store, sess *session.Session, args []string) error {
	question := strings.Join(args, " ")

	if sess == nil {
		id := uuid.New().String()[:8]
		sess = session.NewSession(id, flagModel)
	}
	sess.AddMessage("user", question)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT)
	defer cancel()

	fmt.Fprintln(os.Stderr, render.AssistantStyle.Render(flagModel+":"))

	response, newMsgs, err := chatWithTools(ctx, b, flagModel, sess.Messages, sysPrompt, streamCallback())
	if !flagRender {
		fmt.Println()
	}

	if response != "" || len(newMsgs) > 0 {
		// Add intermediate tool messages to session.
		for _, m := range newMsgs {
			sess.Messages = append(sess.Messages, m)
		}
		if response != "" {
			if flagRender {
				fmt.Print(render.RenderMarkdown(response))
			}
			sess.AddMessage("assistant", response)
		}
		if saveErr := store.Save(sess); saveErr != nil {
			render.Errorf("Failed to save session: %v", saveErr)
		}
	}

	return err
}

func runREPL(b backend.Backend, sysPrompt string, store *session.Store, sess *session.Session) error {
	if sess == nil {
		id := uuid.New().String()[:8]
		sess = session.NewSession(id, flagModel)
	}

	render.PrintLemonColored(render.LemonIdle)
	render.Infof("localfreshllm — model: %s — /help for commands, /quit to exit", flagModel)
	fmt.Println()

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          render.UserStyle.Render("You: "),
		HistoryFile:     historyFile(),
		InterruptPrompt: "^C",
		EOFPrompt:       "/quit",
	})
	if err != nil {
		return fmt.Errorf("readline init: %w", err)
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			break
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		// Slash commands.
		if strings.HasPrefix(input, "/") {
			if handleSlashCommand(input, sess, store, &b, rl) {
				return nil // /quit
			}
			continue
		}

		sess.AddMessage("user", input)

		fmt.Println(render.AssistantStyle.Render(flagModel + ":"))

		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT)

		response, newMsgs, err := chatWithTools(ctx, b, flagModel, sess.Messages, sysPrompt, streamCallback())
		cancel()
		if !flagRender {
			fmt.Println()
		}
		fmt.Println()

		if response != "" || len(newMsgs) > 0 {
			// Add intermediate tool messages to session.
			for _, m := range newMsgs {
				sess.Messages = append(sess.Messages, m)
			}
			if response != "" {
				if flagRender {
					fmt.Print(render.RenderMarkdown(response))
					fmt.Println()
				}
				sess.AddMessage("assistant", response)
			}
			if saveErr := store.Save(sess); saveErr != nil {
				render.Errorf("Failed to save session: %v", saveErr)
			}
		}

		if err != nil && ctx.Err() == nil {
			fmt.Println(render.ErrorStyle.Render(fmt.Sprintf("Error: %v", err)))
		}
	}

	return nil
}

// handleSlashCommand processes REPL commands. Returns true if the REPL should exit.
func handleSlashCommand(input string, sess *session.Session, store *session.Store, b *backend.Backend, rl *readline.Instance) bool {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/quit", "/exit", "/q":
		return true

	case "/model":
		models := listAllModels()
		var chosen string
		if len(parts) >= 2 {
			arg := parts[1]
			// Try as a number first.
			if n, err := strconv.Atoi(arg); err == nil && n >= 1 && n <= len(models) {
				chosen = models[n-1]
			} else {
				chosen = arg
			}
		} else {
			// Interactive model picker.
			if len(models) == 0 {
				render.Infof("No models available.")
				return false
			}
			fmt.Println(render.AssistantStyle.Render("Select a model:"))
			for i, m := range models {
				marker := "  "
				if m == flagModel {
					marker = render.UserStyle.Render("> ")
				}
				fmt.Printf("%s%s %s\n", marker, render.DimStyle.Render(fmt.Sprintf("[%d]", i+1)), render.ModelStyle.Render(m))
			}
			rl.SetPrompt(render.SystemStyle.Render("Enter number: "))
			choice, err := rl.Readline()
			rl.SetPrompt(render.UserStyle.Render("You: "))
			if err != nil {
				return false
			}
			n, nerr := strconv.Atoi(strings.TrimSpace(choice))
			if nerr != nil || n < 1 || n > len(models) {
				render.Infof("Invalid selection.")
				return false
			}
			chosen = models[n-1]
		}
		if chosen != "" {
			newBackend := backend.ForModel(chosen)
			if err := newBackend.Validate(); err != nil {
				fmt.Println(render.ErrorStyle.Render(fmt.Sprintf("Error: %v", err)))
				return false
			}
			flagModel = chosen
			*b = newBackend
			sess.Model = flagModel
			cfg.Model = flagModel
			if err := cfg.Save(); err != nil {
				render.Errorf("Failed to save config: %v", err)
			}
			render.Infof("Switched to %s (saved as default)", flagModel)
		}

	case "/clear":
		sess.Messages = nil
		render.Infof("Conversation cleared.")

	case "/history":
		if err := runHistory(); err != nil {
			render.Errorf("Error: %v", err)
		}

	case "/tools":
		flagTools = !flagTools
		if flagTools {
			render.Infof("Tools enabled")
		} else {
			render.Infof("Tools disabled")
		}

	case "/location":
		if len(parts) >= 2 {
			loc := strings.Join(parts[1:], " ")
			cfg.Location = loc
			if err := cfg.Save(); err != nil {
				render.Errorf("Failed to save config: %v", err)
			} else {
				render.Infof("Location set to %s", loc)
				// Prefetch weather for the new location in the background.
				weather.NewClient().Prefetch(loc)
			}
		} else if cfg.Location != "" {
			render.Infof("Current location: %s", cfg.Location)
			render.Infof("Usage: /location <city or zip>")
		} else {
			render.Infof("No location set. Usage: /location <city or zip>")
		}

	case "/help":
		fmt.Println(render.SystemStyle.Render("Commands:"))
		fmt.Println(render.SystemStyle.Render("  /model         — pick model from list"))
		fmt.Println(render.SystemStyle.Render("  /model <name>  — switch to named model"))
		fmt.Println(render.SystemStyle.Render("  /location      — show or set default location"))
		fmt.Println(render.SystemStyle.Render("  /clear         — clear conversation"))
		fmt.Println(render.SystemStyle.Render("  /history       — list saved sessions"))
		fmt.Println(render.SystemStyle.Render("  /tools         — toggle web search tools"))
		fmt.Println(render.SystemStyle.Render("  /quit          — exit"))

	default:
		render.Infof("Unknown command: %s (try /help)", cmd)
	}

	return false
}

func streamCallback() backend.StreamCallback {
	if flagRender {
		// Buffer mode: collect tokens, render at end.
		return nil
	}
	return func(token string) {
		fmt.Print(token)
	}
}

// listAllModels queries both backends and returns a combined model list.
func listAllModels() []string {
	return service.ListModels(context.Background())
}
