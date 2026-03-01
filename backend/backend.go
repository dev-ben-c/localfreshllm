package backend

import "context"

// StreamCallback is called for each token during streaming.
type StreamCallback func(token string)

// ToolCall represents an LLM's request to invoke a tool.
type ToolCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// ContentBlock represents a typed block within a message (text or tool result).
type ContentBlock struct {
	Type      string `json:"type"`                 // "text", "tool_result", "tool_use"
	Text      string `json:"text,omitempty"`        // for type="text"
	ToolUseID string `json:"tool_use_id,omitempty"` // for type="tool_result"
	Content   string `json:"content,omitempty"`     // for type="tool_result"
	IsError   bool   `json:"is_error,omitempty"`    // for type="tool_result"
}

// Message represents a chat message.
type Message struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []ToolCall     `json:"tool_calls,omitempty"`
	Blocks    []ContentBlock `json:"blocks,omitempty"` // structured content (tool results)
}

// ChatResult holds the output of a Chat call.
type ChatResult struct {
	Text      string
	ToolCalls []ToolCall
}

// Backend defines the interface for LLM providers.
type Backend interface {
	Chat(ctx context.Context, model string, messages []Message, systemPrompt string, toolDefs []any, onToken StreamCallback) (*ChatResult, error)
	ListModels(ctx context.Context) ([]string, error)
	// Validate checks if the backend is ready (e.g., API key set, server reachable).
	// Returns nil if ready, or a user-friendly error message.
	Validate() error
}

// ForModel returns the appropriate backend based on model name prefix.
// Models starting with "claude-" use Anthropic, everything else uses Ollama.
func ForModel(model string) Backend {
	if len(model) >= 7 && model[:7] == "claude-" {
		return NewAnthropic()
	}
	return NewOllama()
}
