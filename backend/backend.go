package backend

import "context"

// StreamCallback is called for each token during streaming.
type StreamCallback func(token string)

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Backend defines the interface for LLM providers.
type Backend interface {
	Chat(ctx context.Context, model string, messages []Message, systemPrompt string, onToken StreamCallback) (string, error)
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
