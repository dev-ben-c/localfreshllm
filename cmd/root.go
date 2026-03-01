package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/rabidclock/localfreshllm/backend"
	"github.com/rabidclock/localfreshllm/client"
	"github.com/rabidclock/localfreshllm/render"
	"github.com/rabidclock/localfreshllm/service"
	"github.com/rabidclock/localfreshllm/session"
	"github.com/rabidclock/localfreshllm/systemprompt"
	"github.com/rabidclock/localfreshllm/tui"
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

	// TUI mode: interactive.
	return runTUI(b, sysPrompt, store, sess, false)
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

	// TUI mode — no local session save in client mode.
	return runTUI(remote, sysPrompt, nil, nil, true)
}

// runTUI launches the Bubble Tea TUI for interactive chat.
func runTUI(b backend.Backend, sysPrompt string, store *session.Store, sess *session.Session, isClient bool) error {
	if sess == nil && !isClient {
		id := uuid.New().String()[:8]
		sess = session.NewSession(id, flagModel)
	}

	m := tui.New(tui.Config{
		Backend:      b,
		ChatService:  chatService,
		Store:        store,
		Session:      sess,
		UserConfig:   cfg,
		Model:        flagModel,
		SystemPrompt: sysPrompt,
		EnableTools:  flagTools,
		RenderMD:     flagRender,
		IsClient:     isClient,
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
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

// cliEmit handles ChatEvents for CLI output (one-shot / pipe modes).
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
