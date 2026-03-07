package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dev-ben-c/localfreshllm/backend"
)

// RemoteBackend implements backend.Backend by proxying to a LocalFreshLLM server.
type RemoteBackend struct {
	serverURL  string
	apiKey     string
	httpClient *http.Client
	sessionID  string // tracks the active server-side session
}

// New creates a new remote backend.
func New(serverURL, apiKey string) *RemoteBackend {
	return &RemoteBackend{
		serverURL:  strings.TrimRight(serverURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

// chatRequest matches the server's expected JSON body.
type chatRequest struct {
	Message     string `json:"message"`
	SessionID   string `json:"session_id,omitempty"`
	Model       string `json:"model,omitempty"`
	EnableTools *bool  `json:"enable_tools,omitempty"`
}

// Chat sends a message to the server and streams the response via SSE.
// Tool execution happens server-side — toolDefs is ignored.
func (r *RemoteBackend) Chat(ctx context.Context, model string, messages []backend.Message, systemPrompt string, toolDefs []any, onToken backend.StreamCallback) (*backend.ChatResult, error) {
	// Extract the last user message as the message to send.
	// The server manages its own session history.
	var lastUserMsg string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserMsg = messages[i].Content
			break
		}
	}
	if lastUserMsg == "" {
		return nil, fmt.Errorf("no user message found")
	}

	enableTools := toolDefs != nil
	body := chatRequest{
		Message:     lastUserMsg,
		SessionID:   r.sessionID,
		Model:       model,
		EnableTools: &enableTools,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.serverURL+"/v1/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("server request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, errResp.Error)
		}
		return nil, fmt.Errorf("server error: %d", resp.StatusCode)
	}

	// Parse SSE stream.
	scanner := bufio.NewScanner(resp.Body)
	var finalText string
	var lastError string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			eventType := strings.TrimPrefix(line, "event: ")

			// Read the data line.
			if !scanner.Scan() {
				break
			}
			dataLine := scanner.Text()
			if !strings.HasPrefix(dataLine, "data: ") {
				continue
			}
			data := strings.TrimPrefix(dataLine, "data: ")

			switch eventType {
			case "token":
				var ev struct {
					Text string `json:"text"`
				}
				if json.Unmarshal([]byte(data), &ev) == nil && onToken != nil {
					onToken(ev.Text)
				}

			case "tool_call":
				// Tool calls are handled server-side. We could display them
				// but the service layer emit callback handles that.

			case "done":
				var ev struct {
					Text      string `json:"text"`
					SessionID string `json:"session_id"`
				}
				if json.Unmarshal([]byte(data), &ev) == nil {
					finalText = ev.Text
					if ev.SessionID != "" {
						r.sessionID = ev.SessionID
					}
				}

			case "error":
				var ev struct {
					Text string `json:"text"`
				}
				if json.Unmarshal([]byte(data), &ev) == nil {
					lastError = ev.Text
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read SSE stream: %w", err)
	}

	if lastError != "" {
		return &backend.ChatResult{Text: finalText}, fmt.Errorf("server: %s", lastError)
	}

	return &backend.ChatResult{Text: finalText}, nil
}

// ListModels queries the server for available models.
func (r *RemoteBackend) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.serverURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list models: status %d", resp.StatusCode)
	}

	var result struct {
		Models []string `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse models: %w", err)
	}

	return result.Models, nil
}

// ClearSession resets the tracked session ID so the next chat starts fresh.
func (r *RemoteBackend) ClearSession() {
	r.sessionID = ""
}

// UpdateLocation sets the device's location on the server.
func (r *RemoteBackend) UpdateLocation(location string) error {
	body, _ := json.Marshal(map[string]string{"location": location})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, r.serverURL+"/v1/devices/me", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("update location: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update location: status %d", resp.StatusCode)
	}
	return nil
}

// Validate checks connectivity to the server.
func (r *RemoteBackend) Validate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.serverURL+"/health", nil)
	if err != nil {
		return err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot connect to server at %s: %w", r.serverURL, err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server health check failed: %d", resp.StatusCode)
	}

	return nil
}
