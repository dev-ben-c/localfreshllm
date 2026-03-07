package shell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// ToolCall represents an LLM's request to invoke a tool.
type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// ToolResult is the output returned to the LLM after executing a tool.
type ToolResult struct {
	ID      string
	Name    string
	Content string
	IsError bool
}

type toolDef struct {
	Name        string
	Description string
	Properties  map[string]map[string]string
	Required    []string
}

const (
	defaultTimeout = 30  // seconds
	maxTimeout     = 300 // seconds
	maxOutput      = 100 * 1024 // 100 KB
)

var allTools = []toolDef{
	{
		Name:        "shell_exec",
		Description: "Execute a shell command and return its output. Commands run as the server user. Use 'sudo' for privileged operations. Returns combined stdout and stderr.",
		Properties: map[string]map[string]string{
			"command": {"type": "string", "description": "The shell command to execute (passed to bash -c)"},
			"timeout": {"type": "integer", "description": "Timeout in seconds (optional, default 30, max 300)"},
		},
		Required: []string{"command"},
	},
}

func buildProps(props map[string]map[string]string) map[string]any {
	out := make(map[string]any, len(props))
	for name, p := range props {
		out[name] = map[string]any{"type": p["type"], "description": p["description"]}
	}
	return out
}

// OllamaToolDefs returns tool definitions in Ollama's format.
func OllamaToolDefs() []any {
	out := make([]any, 0, len(allTools))
	for _, t := range allTools {
		params := map[string]any{
			"type":       "object",
			"properties": buildProps(t.Properties),
		}
		if len(t.Required) > 0 {
			params["required"] = t.Required
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  params,
			},
		})
	}
	return out
}

// AnthropicToolDefs returns tool definitions in Anthropic's format.
func AnthropicToolDefs() []any {
	out := make([]any, 0, len(allTools))
	for _, t := range allTools {
		schema := map[string]any{
			"type":       "object",
			"properties": buildProps(t.Properties),
		}
		if len(t.Required) > 0 {
			schema["required"] = t.Required
		}
		out = append(out, map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": schema,
		})
	}
	return out
}

// ToolNames returns the set of tool names provided by this package.
func ToolNames() map[string]bool {
	names := make(map[string]bool, len(allTools))
	for _, t := range allTools {
		names[t.Name] = true
	}
	return names
}

// IsEnabled returns true if SHELL_ENABLED=true in environment.
func IsEnabled() bool {
	return strings.EqualFold(os.Getenv("SHELL_ENABLED"), "true")
}

// Executor runs shell tool calls.
type Executor struct {
	sudoPassword string // set per-request, never logged
}

// NewExecutor creates a shell tool executor.
func NewExecutor() *Executor {
	return &Executor{}
}

// WithSudoPassword returns a copy of the executor with the given sudo password set.
func (e *Executor) WithSudoPassword(password string) *Executor {
	return &Executor{sudoPassword: password}
}

// Execute runs a single tool call and returns the result.
func (e *Executor) Execute(ctx context.Context, call ToolCall) ToolResult {
	var result ToolResult
	switch call.Name {
	case "shell_exec":
		result = e.execShell(ctx, call)
	default:
		return ToolResult{
			ID:      call.ID,
			Name:    call.Name,
			Content: fmt.Sprintf("unknown tool: %s", call.Name),
			IsError: true,
		}
	}

	result.Content = sanitize(result.Content)
	if !result.IsError {
		result.Content = "[TOOL RESULT: " + result.Name + "]\n" + result.Content + "\n[END TOOL RESULT]"
	}
	return result
}

func (e *Executor) execShell(ctx context.Context, call ToolCall) ToolResult {
	command, _ := call.Args["command"].(string)
	if command == "" {
		return ToolResult{ID: call.ID, Name: call.Name, Content: "missing required parameter: command", IsError: true}
	}

	timeout := defaultTimeout
	if t, ok := toInt(call.Args["timeout"]); ok && t > 0 {
		timeout = t
		if timeout > maxTimeout {
			timeout = maxTimeout
		}
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// If we have a cached sudo password and the command uses sudo,
	// prepend a credential-caching step so subsequent sudo calls work.
	runCmd := command
	var stdin io.Reader
	if e.sudoPassword != "" && containsSudo(command) {
		runCmd = "sudo -S -v 2>/dev/null && " + command
		stdin = strings.NewReader(e.sudoPassword + "\n")
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", runCmd)
	cmd.Stdin = stdin // nil = /dev/null when no password
	cmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()

	output := buf.String()
	truncated := false
	if len(output) > maxOutput {
		output = output[:maxOutput]
		truncated = true
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "$ %s\n", command)

	if output != "" {
		sb.WriteString(output)
		if !strings.HasSuffix(output, "\n") {
			sb.WriteByte('\n')
		}
	}

	if truncated {
		sb.WriteString("... (output truncated at 100 KB)\n")
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Fprintf(&sb, "\n[timed out after %ds]", timeout)
			return ToolResult{ID: call.ID, Name: call.Name, Content: sb.String(), IsError: true}
		}
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		fmt.Fprintf(&sb, "\n[exit code %d]", exitCode)
		return ToolResult{ID: call.ID, Name: call.Name, Content: sb.String(), IsError: true}
	}

	sb.WriteString("\n[exit code 0]")
	return ToolResult{ID: call.ID, Name: call.Name, Content: sb.String()}
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}

var sudoPattern = regexp.MustCompile(`\bsudo\b`)

// containsSudo checks if the command contains the word "sudo".
func containsSudo(command string) bool {
	return sudoPattern.MatchString(command)
}

// validateSudo tests whether a password works for sudo on this system.
func validateSudo(password string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sudo", "-S", "-v")
	cmd.Stdin = strings.NewReader(password + "\n")
	cmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("sudo authentication failed: %s", msg)
	}
	return nil
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
