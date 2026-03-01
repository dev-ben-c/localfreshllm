package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/rabidclock/localfreshllm/backend"
	"github.com/rabidclock/localfreshllm/render"
	"github.com/rabidclock/localfreshllm/session"
	"github.com/rabidclock/localfreshllm/systemprompt"
)

var (
	flagModel   string
	flagSystem  string
	flagPersona string
	flagList    bool
	flagHistory bool
	flagResume  string
	flagRender  bool
)

var rootCmd = &cobra.Command{
	Use:   "localfreshllm [question]",
	Short: "CLI for local and cloud LLMs",
	Long:  "Talk to Ollama models and Claude from the terminal. Streaming, sessions, and pipe support.",
	// Suppress cobra's default error/usage on RunE errors.
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE:          run,
}

func init() {
	rootCmd.Flags().StringVarP(&flagModel, "model", "m", "claude-opus-4-6", "Model name")
	rootCmd.Flags().StringVarP(&flagSystem, "system", "s", "", "Custom system prompt")
	rootCmd.Flags().StringVarP(&flagPersona, "persona", "p", "", "Named preset (coder, reviewer, writer, shell)")
	rootCmd.Flags().BoolVar(&flagList, "list", false, "List available models")
	rootCmd.Flags().BoolVar(&flagHistory, "history", false, "List saved conversations")
	rootCmd.Flags().StringVar(&flagResume, "resume", "", "Session ID (or prefix) to continue")
	rootCmd.Flags().BoolVar(&flagRender, "render", false, "Render markdown output with glamour")
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func run(cmd *cobra.Command, args []string) error {
	// Subcommand modes.
	if flagList {
		return runList()
	}
	if flagHistory {
		return runHistory()
	}

	sysPrompt := systemprompt.Get(flagSystem, flagPersona)
	b := backend.ForModel(flagModel)
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

	_, chatErr := b.Chat(ctx, flagModel, messages, sysPrompt, func(token string) {
		fmt.Print(token)
	})
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

	response, err := b.Chat(ctx, flagModel, sess.Messages, sysPrompt, streamCallback())
	if !flagRender {
		fmt.Println()
	}

	if response != "" {
		if flagRender {
			fmt.Print(render.RenderMarkdown(response))
		}
		sess.AddMessage("assistant", response)
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

	render.Infof("localfreshllm — model: %s — /help for commands, /quit to exit", flagModel)
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(render.UserStyle.Render("You: "))
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Slash commands.
		if strings.HasPrefix(input, "/") {
			if handleSlashCommand(input, sess, store, &b) {
				return nil // /quit
			}
			continue
		}

		sess.AddMessage("user", input)

		fmt.Println(render.AssistantStyle.Render(flagModel + ":"))

		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT)

		response, err := b.Chat(ctx, flagModel, sess.Messages, sysPrompt, streamCallback())
		cancel()
		if !flagRender {
			fmt.Println()
		}
		fmt.Println()

		if response != "" {
			if flagRender {
				fmt.Print(render.RenderMarkdown(response))
				fmt.Println()
			}
			sess.AddMessage("assistant", response)
			if saveErr := store.Save(sess); saveErr != nil {
				render.Errorf("Failed to save session: %v", saveErr)
			}
		}

		if err != nil && ctx.Err() == nil {
			render.Errorf("Error: %v", err)
		}
	}

	return nil
}

// handleSlashCommand processes REPL commands. Returns true if the REPL should exit.
func handleSlashCommand(input string, sess *session.Session, store *session.Store, b *backend.Backend) bool {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/quit", "/exit", "/q":
		return true

	case "/model":
		if len(parts) < 2 {
			render.Infof("Current model: %s", flagModel)
			return false
		}
		flagModel = parts[1]
		*b = backend.ForModel(flagModel)
		sess.Model = flagModel
		render.Infof("Switched to %s", flagModel)

	case "/clear":
		sess.Messages = nil
		render.Infof("Conversation cleared.")

	case "/history":
		if err := runHistory(); err != nil {
			render.Errorf("Error: %v", err)
		}

	case "/help":
		fmt.Println(render.SystemStyle.Render("Commands:"))
		fmt.Println(render.SystemStyle.Render("  /model <name>  — switch model"))
		fmt.Println(render.SystemStyle.Render("  /clear         — clear conversation"))
		fmt.Println(render.SystemStyle.Render("  /history       — list saved sessions"))
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
