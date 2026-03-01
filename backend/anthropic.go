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

func (a *Anthropic) Validate() error {
	if a.apiKey == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY is not set.\n  export ANTHROPIC_API_KEY=sk-ant-...\n  Or use a local model: localfreshllm -m qwen2.5:7b")
	}
	return nil
}

type anthropicRequest struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	Stream    bool   `json:"stream"`
	System    string `json:"system,omitempty"`
	Messages  []any  `json:"messages"`
	Tools     []any  `json:"tools,omitempty"`
}

type anthropicSSEEvent struct {
	Type    string          `json:"type"`
	Index   int             `json:"index,omitempty"`
	Delta   json.RawMessage `json:"delta,omitempty"`
	Content json.RawMessage `json:"content_block,omitempty"`
}

type anthropicDelta struct {
	Type         string `json:"type"`
	Text         string `json:"text"`
	PartialJSON  string `json:"partial_json,omitempty"`
	StopReason   string `json:"stop_reason,omitempty"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
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

// anthropicMarshalMessages converts backend.Message to Anthropic's wire format.
func anthropicMarshalMessages(messages []Message) []any {
	out := make([]any, 0, len(messages))
	for _, m := range messages {
		if m.Role == "system" {
			continue
		}

		// Messages with structured blocks (tool results).
		if len(m.Blocks) > 0 {
			blocks := make([]map[string]any, 0, len(m.Blocks))
			for _, b := range m.Blocks {
				switch b.Type {
				case "tool_result":
					block := map[string]any{
						"type":        "tool_result",
						"tool_use_id": b.ToolUseID,
						"content":     b.Content,
					}
					if b.IsError {
						block["is_error"] = true
					}
					blocks = append(blocks, block)
				case "text":
					blocks = append(blocks, map[string]any{
						"type": "text",
						"text": b.Text,
					})
				}
			}
			out = append(out, map[string]any{
				"role":    m.Role,
				"content": blocks,
			})
			continue
		}

		// Assistant messages with tool calls.
		if len(m.ToolCalls) > 0 {
			blocks := make([]map[string]any, 0, len(m.ToolCalls)+1)
			if m.Content != "" {
				blocks = append(blocks, map[string]any{
					"type": "text",
					"text": m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": tc.Args,
				})
			}
			out = append(out, map[string]any{
				"role":    "assistant",
				"content": blocks,
			})
			continue
		}

		// Plain text message.
		out = append(out, map[string]any{
			"role":    m.Role,
			"content": m.Content,
		})
	}
	return out
}

func (a *Anthropic) Chat(ctx context.Context, model string, messages []Message, systemPrompt string, toolDefs []any, onToken StreamCallback) (*ChatResult, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set. Export it or use -m for a local model.")
	}

	body := anthropicRequest{
		Model:     model,
		MaxTokens: 8192,
		Stream:    true,
		System:    systemPrompt,
		Messages:  anthropicMarshalMessages(messages),
		Tools:     toolDefs,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		switch resp.StatusCode {
		case 401:
			return nil, fmt.Errorf("invalid API key. Check your ANTHROPIC_API_KEY.")
		case 429:
			return nil, fmt.Errorf("rate limited. Wait a moment and try again.")
		case 529:
			return nil, fmt.Errorf("anthropic API is overloaded. Try again shortly.")
		default:
			var apiErr anthropicError
			if json.Unmarshal(errBody, &apiErr) == nil && apiErr.Error.Message != "" {
				return nil, fmt.Errorf("anthropic error (%d): %s", resp.StatusCode, apiErr.Error.Message)
			}
			return nil, fmt.Errorf("anthropic error (%d): %s", resp.StatusCode, string(errBody))
		}
	}

	return a.parseSSEStream(ctx, resp.Body, onToken)
}

func (a *Anthropic) parseSSEStream(ctx context.Context, body io.Reader, onToken StreamCallback) (*ChatResult, error) {
	var full bytes.Buffer
	var toolCalls []ToolCall
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Track current tool_use block being built.
	type toolUseState struct {
		id        string
		name      string
		inputJSON bytes.Buffer
	}
	var currentTool *toolUseState

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return &ChatResult{Text: full.String(), ToolCalls: toolCalls}, ctx.Err()
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
		case "content_block_start":
			var block anthropicContentBlock
			if err := json.Unmarshal(event.Content, &block); err != nil {
				continue
			}
			if block.Type == "tool_use" {
				currentTool = &toolUseState{id: block.ID, name: block.Name}
			}

		case "content_block_delta":
			var delta anthropicDelta
			if err := json.Unmarshal(event.Delta, &delta); err != nil {
				continue
			}
			if delta.Type == "text_delta" && delta.Text != "" {
				full.WriteString(delta.Text)
				if onToken != nil {
					onToken(delta.Text)
				}
			}
			if delta.Type == "input_json_delta" && currentTool != nil {
				currentTool.inputJSON.WriteString(delta.PartialJSON)
			}

		case "content_block_stop":
			if currentTool != nil {
				var args map[string]any
				if currentTool.inputJSON.Len() > 0 {
					json.Unmarshal(currentTool.inputJSON.Bytes(), &args)
				}
				if args == nil {
					args = map[string]any{}
				}
				toolCalls = append(toolCalls, ToolCall{
					ID:   currentTool.id,
					Name: currentTool.name,
					Args: args,
				})
				currentTool = nil
			}

		case "message_stop":
			return &ChatResult{Text: full.String(), ToolCalls: toolCalls}, nil

		case "message_delta":
			// Check for stop_reason in message_delta.
			var delta struct {
				StopReason string `json:"stop_reason"`
			}
			if json.Unmarshal(event.Delta, &delta) == nil && delta.StopReason == "tool_use" {
				// Will continue until message_stop.
			}

		case "error":
			var errMsg struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(event.Delta, &errMsg) == nil {
				return &ChatResult{Text: full.String(), ToolCalls: toolCalls}, fmt.Errorf("stream error: %s", errMsg.Message)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return &ChatResult{Text: full.String(), ToolCalls: toolCalls}, fmt.Errorf("read stream: %w", err)
	}

	return &ChatResult{Text: full.String(), ToolCalls: toolCalls}, nil
}

func (a *Anthropic) ListModels(_ context.Context) ([]string, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
	}
	return claudeModels, nil
}
