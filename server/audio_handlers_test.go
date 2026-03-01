package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rabidclock/localfreshllm/audio"
	"github.com/rabidclock/localfreshllm/device"
)

// withDevice injects a device profile into the request context for testing.
func withDevice(r *http.Request) *http.Request {
	profile := &device.Profile{
		ID:    "testdev",
		Name:  "test-device",
		Token: "test-token",
	}
	ctx := context.WithValue(r.Context(), deviceKey, profile)
	return r.WithContext(ctx)
}

func TestHandleTranscribe_NotConfigured(t *testing.T) {
	srv := &Server{} // No whisper client.

	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcribe", bytes.NewReader([]byte{0x01}))
	req.Header.Set("Content-Type", "application/octet-stream")
	req = withDevice(req)

	w := httptest.NewRecorder()
	srv.handleTranscribe(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("not configured")) {
		t.Errorf("expected 'not configured' in response, got %q", w.Body.String())
	}
}

func TestHandleTranscribe_WrongMethod(t *testing.T) {
	srv := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/v1/audio/transcribe", nil)
	req = withDevice(req)

	w := httptest.NewRecorder()
	srv.handleTranscribe(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleTranscribe_EmptyBody(t *testing.T) {
	srv := &Server{
		whisper: audio.NewWhisperClient("http://localhost:99999"), // Won't be called.
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcribe", bytes.NewReader(nil))
	req.Header.Set("Content-Type", "application/octet-stream")
	req = withDevice(req)

	w := httptest.NewRecorder()
	srv.handleTranscribe(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTranscribe_WithMockWhisper(t *testing.T) {
	// Create a mock whisper server.
	mockWhisper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"text": "hello world"})
	}))
	defer mockWhisper.Close()

	srv := &Server{
		whisper: audio.NewWhisperClient(mockWhisper.URL),
	}

	pcm := make([]byte, 100) // Dummy PCM data.
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcribe", bytes.NewReader(pcm))
	req.Header.Set("Content-Type", "application/octet-stream")
	req = withDevice(req)

	w := httptest.NewRecorder()
	srv.handleTranscribe(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Text != "hello world" {
		t.Errorf("expected 'hello world', got %q", result.Text)
	}
}

func TestHandleSpeak_NotConfigured(t *testing.T) {
	srv := &Server{} // No piper.

	body, _ := json.Marshal(map[string]string{"text": "hello"})
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speak", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withDevice(req)

	w := httptest.NewRecorder()
	srv.handleSpeak(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
}

func TestHandleSpeak_WrongMethod(t *testing.T) {
	srv := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/v1/audio/speak", nil)
	req = withDevice(req)

	w := httptest.NewRecorder()
	srv.handleSpeak(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleSpeak_EmptyText(t *testing.T) {
	srv := &Server{
		piper: audio.NewPiperTTS("/nonexistent/model", ""),
	}

	body, _ := json.Marshal(map[string]string{"text": ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speak", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withDevice(req)

	w := httptest.NewRecorder()
	srv.handleSpeak(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSpeak_InvalidJSON(t *testing.T) {
	srv := &Server{
		piper: audio.NewPiperTTS("/nonexistent/model", ""),
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speak", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req = withDevice(req)

	w := httptest.NewRecorder()
	srv.handleSpeak(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
