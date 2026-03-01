package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Transcribe sends raw PCM audio (16kHz, 16-bit mono) to the server for
// speech-to-text transcription.
func (r *RemoteBackend) Transcribe(ctx context.Context, pcmData []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		r.serverURL+"/v1/audio/transcribe",
		bytes.NewReader(pcmData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("transcribe request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			return "", fmt.Errorf("transcribe error (%d): %s", resp.StatusCode, errResp.Error)
		}
		return "", fmt.Errorf("transcribe error: %d", resp.StatusCode)
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	return result.Text, nil
}

// Speak sends text to the server for text-to-speech synthesis.
// Returns raw WAV audio data.
func (r *RemoteBackend) Speak(ctx context.Context, text string) ([]byte, error) {
	reqBody, _ := json.Marshal(map[string]string{"text": text})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		r.serverURL+"/v1/audio/speak",
		bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("speak request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("speak error (%d): %s", resp.StatusCode, errResp.Error)
		}
		return nil, fmt.Errorf("speak error: %d", resp.StatusCode)
	}

	return body, nil
}
