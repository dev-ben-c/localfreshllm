package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
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

	"github.com/dev-ben-c/localfreshsearch/tools"
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
	rootCmd.Flags().BoolVar(&flagTools, "tools", true, "Enable web search and page reading tools")
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

// getToolDefs returns the appropriate tool definitions for the current backend,
// or nil if tools are disabled.
func getToolDefs() []any {
	if !flagTools {
		return nil
	}
	if len(flagModel) >= 7 && flagModel[:7] == "claude-" {
		return tools.AnthropicToolDefs()
	}
	return tools.OllamaToolDefs()
}

// chatWithTools runs a Chat call and loops on tool calls (max 5 iterations).
// It prints [tool: name] to stderr for each tool invocation.
// Returns the final text response.
func chatWithTools(ctx context.Context, b backend.Backend, model string, messages []backend.Message, sysPrompt string, onToken backend.StreamCallback) (string, []backend.Message, error) {
	toolDefs := getToolDefs()

	var executor *tools.Executor
	if toolDefs != nil {
		executor = tools.NewExecutor()
		defer executor.Close()
	}

	// Track new messages generated during tool loop (for session persistence).
	var newMessages []backend.Message

	for i := 0; i < 5; i++ {
		result, err := b.Chat(ctx, model, messages, sysPrompt, toolDefs, onToken)
		if err != nil {
			return result.Text, newMessages, err
		}

		if len(result.ToolCalls) == 0 || toolDefs == nil {
			return result.Text, newMessages, nil
		}

		// Record the assistant's tool-call message.
		assistantMsg := backend.Message{
			Role:      "assistant",
			Content:   result.Text,
			ToolCalls: result.ToolCalls,
		}
		messages = append(messages, assistantMsg)
		newMessages = append(newMessages, assistantMsg)

		// Execute each tool call.
		isAnthropic := len(model) >= 7 && model[:7] == "claude-"

		if isAnthropic {
			// Anthropic: all tool results in one user message with blocks.
			var blocks []backend.ContentBlock
			for _, tc := range result.ToolCalls {
				fmt.Fprintf(os.Stderr, "%s\n", render.DimStyle.Render(fmt.Sprintf("[tool: %s]", tc.Name)))
				tr := executor.Execute(ctx, tools.ToolCall{ID: tc.ID, Name: tc.Name, Args: tc.Args})
				blocks = append(blocks, backend.ContentBlock{
					Type:      "tool_result",
					ToolUseID: tr.ID,
					Content:   tr.Content,
					IsError:   tr.IsError,
				})
			}
			toolMsg := backend.Message{Role: "user", Blocks: blocks}
			messages = append(messages, toolMsg)
			newMessages = append(newMessages, toolMsg)
		} else {
			// Ollama: one tool message per result.
			for _, tc := range result.ToolCalls {
				fmt.Fprintf(os.Stderr, "%s\n", render.DimStyle.Render(fmt.Sprintf("[tool: %s]", tc.Name)))
				tr := executor.Execute(ctx, tools.ToolCall{ID: tc.ID, Name: tc.Name, Args: tc.Args})
				toolMsg := backend.Message{Role: "tool", Content: tr.Content}
				messages = append(messages, toolMsg)
				newMessages = append(newMessages, toolMsg)
			}
		}
	}

	// Hit max iterations — do a final call without tools.
	result, err := b.Chat(ctx, model, messages, sysPrompt, nil, onToken)
	if err != nil {
		return result.Text, newMessages, err
	}
	return result.Text, newMessages, nil
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
			if handleSlashCommand(input, sess, store, &b, scanner) {
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
func handleSlashCommand(input string, sess *session.Session, store *session.Store, b *backend.Backend, scanner *bufio.Scanner) bool {
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
			fmt.Print(render.SystemStyle.Render("Enter number: "))
			if scanner.Scan() {
				choice := strings.TrimSpace(scanner.Text())
				n, err := strconv.Atoi(choice)
				if err != nil || n < 1 || n > len(models) {
					render.Infof("Invalid selection.")
					return false
				}
				chosen = models[n-1]
			}
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
			render.Infof("Switched to %s", flagModel)
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

	case "/help":
		fmt.Println(render.SystemStyle.Render("Commands:"))
		fmt.Println(render.SystemStyle.Render("  /model         — pick model from list"))
		fmt.Println(render.SystemStyle.Render("  /model <name>  — switch to named model"))
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
	ctx := context.Background()
	var all []string

	ollama := backend.NewOllama()
	if models, err := ollama.ListModels(ctx); err == nil {
		all = append(all, models...)
	}

	anthropic := backend.NewAnthropic()
	if models, err := anthropic.ListModels(ctx); err == nil {
		all = append(all, models...)
	}

	return all
}
