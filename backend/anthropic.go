package backend

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Anthropic implements the Backend interface for Claude models.
type Anthropic struct {
	apiKey string
	client *http.Client
}

// NewAnthropic creates an Anthropic backend using ANTHROPIC_API_KEY.
func NewAnthropic() *Anthropic {
	return &Anthropic{
		apiKey: os.Getenv("ANTHROPIC_API_KEY"),
		client: &http.Client{},
	}
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool               `json:"stream"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicSSEEvent struct {
	Type  string          `json:"type"`
	Delta json.RawMessage `json:"delta,omitempty"`
}

type anthropicDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Available Claude models.
var claudeModels = []string{
	"claude-opus-4-6",
	"claude-sonnet-4-6",
	"claude-haiku-4-5-20251001",
}

func (a *Anthropic) Chat(ctx context.Context, model string, messages []Message, systemPrompt string, onToken StreamCallback) (string, error) {
	if a.apiKey == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY is not set. Export it or use -m for a local model.")
	}

	// Convert messages to Anthropic format (only user/assistant roles).
	var apiMessages []anthropicMessage
	for _, m := range messages {
		if m.Role == "system" {
			continue
		}
		apiMessages = append(apiMessages, anthropicMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	body := anthropicRequest{
		Model:     model,
		MaxTokens: 8192,
		Stream:    true,
		System:    systemPrompt,
		Messages:  apiMessages,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		switch resp.StatusCode {
		case 401:
			return "", fmt.Errorf("invalid API key. Check your ANTHROPIC_API_KEY.")
		case 429:
			return "", fmt.Errorf("rate limited. Wait a moment and try again.")
		case 529:
			return "", fmt.Errorf("anthropic API is overloaded. Try again shortly.")
		default:
			var apiErr anthropicError
			if json.Unmarshal(errBody, &apiErr) == nil && apiErr.Error.Message != "" {
				return "", fmt.Errorf("anthropic error (%d): %s", resp.StatusCode, apiErr.Error.Message)
			}
			return "", fmt.Errorf("anthropic error (%d): %s", resp.StatusCode, string(errBody))
		}
	}

	return a.parseSSEStream(ctx, resp.Body, onToken)
}

func (a *Anthropic) parseSSEStream(ctx context.Context, body io.Reader, onToken StreamCallback) (string, error) {
	var full bytes.Buffer
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return full.String(), ctx.Err()
		default:
		}

		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := line[6:]
		if data == "[DONE]" {
			break
		}

		var event anthropicSSEEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_delta":
			var delta anthropicDelta
			if err := json.Unmarshal(event.Delta, &delta); err != nil {
				continue
			}
			if delta.Text != "" {
				full.WriteString(delta.Text)
				if onToken != nil {
					onToken(delta.Text)
				}
			}
		case "message_stop":
			return full.String(), nil
		case "error":
			var errMsg struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(event.Delta, &errMsg) == nil {
				return full.String(), fmt.Errorf("stream error: %s", errMsg.Message)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return full.String(), fmt.Errorf("read stream: %w", err)
	}

	return full.String(), nil
}

func (a *Anthropic) ListModels(_ context.Context) ([]string, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
	}
	return claudeModels, nil
}
