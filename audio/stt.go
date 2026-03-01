package audio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// WhisperClient talks to a whisper.cpp HTTP server for speech-to-text.
type WhisperClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewWhisperClient creates a client pointing at the given whisper.cpp server URL.
func NewWhisperClient(baseURL string) *WhisperClient {
	return &WhisperClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Transcribe sends raw PCM audio (16kHz, 16-bit mono) to the whisper server
// and returns the transcribed text.
func (w *WhisperClient) Transcribe(ctx context.Context, pcmData []byte) (string, error) {
	// Whisper server expects a WAV file.
	wavData := WriteWAVHeader(pcmData, 16000)

	// Build multipart form.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(wavData); err != nil {
		return "", fmt.Errorf("write wav data: %w", err)
	}

	// Set response format to JSON.
	if err := writer.WriteField("response_format", "json"); err != nil {
		return "", fmt.Errorf("write field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.BaseURL+"/inference", &body)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := w.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("whisper request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("whisper server error (%d): %s", resp.StatusCode, string(respBody))
	}

	// Parse JSON response.
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		// Some whisper builds return plain text.
		return string(bytes.TrimSpace(respBody)), nil
	}

	return result.Text, nil
}
