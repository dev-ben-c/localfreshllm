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
	"sort"
)

// Ollama implements the Backend interface for local Ollama models.
type Ollama struct {
	host   string
	client *http.Client
}

// NewOllama creates an Ollama backend. Respects OLLAMA_HOST env var.
func NewOllama() *Ollama {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://127.0.0.1:11434"
	}
	return &Ollama{
		host:   host,
		client: &http.Client{},
	}
}

type ollamaChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type ollamaChatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func (o *Ollama) Chat(ctx context.Context, model string, messages []Message, systemPrompt string, onToken StreamCallback) (string, error) {
	// Prepend system prompt as a system message.
	var allMessages []Message
	if systemPrompt != "" {
		allMessages = append(allMessages, Message{Role: "system", Content: systemPrompt})
	}
	allMessages = append(allMessages, messages...)

	body := ollamaChatRequest{
		Model:    model,
		Messages: allMessages,
		Stream:   true,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.host+"/api/chat", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot connect to Ollama at %s. Is `ollama serve` running?", o.host)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama error (%d): %s", resp.StatusCode, string(errBody))
	}

	var full bytes.Buffer
	scanner := bufio.NewScanner(resp.Body)
	// Increase scanner buffer for large responses.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return full.String(), ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var chunk ollamaChatResponse
		if err := json.Unmarshal(line, &chunk); err != nil {
			continue
		}

		if chunk.Message.Content != "" {
			full.WriteString(chunk.Message.Content)
			if onToken != nil {
				onToken(chunk.Message.Content)
			}
		}

		if chunk.Done {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return full.String(), fmt.Errorf("read stream: %w", err)
	}

	return full.String(), nil
}

func (o *Ollama) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", o.host+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Ollama at %s. Is `ollama serve` running?", o.host)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama error (%d)", resp.StatusCode)
	}

	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	models := make([]string, 0, len(tags.Models))
	for _, m := range tags.Models {
		models = append(models, m.Name)
	}
	sort.Strings(models)
	return models, nil
}
