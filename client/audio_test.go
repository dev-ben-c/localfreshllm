package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranscribe_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/audio/transcribe" {
			t.Errorf("expected /v1/audio/transcribe, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Errorf("expected application/octet-stream, got %s", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"text": "transcribed text"})
	}))
	defer server.Close()

	rb := New(server.URL, "test-key")
	text, err := rb.Transcribe(context.Background(), []byte{0x01, 0x02})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "transcribed text" {
		t.Errorf("expected 'transcribed text', got %q", text)
	}
}

func TestTranscribe_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "whisper failed"})
	}))
	defer server.Close()

	rb := New(server.URL, "test-key")
	_, err := rb.Transcribe(context.Background(), []byte{0x01})
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestTranscribe_NotImplemented(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
		json.NewEncoder(w).Encode(map[string]string{"error": "not configured"})
	}))
	defer server.Close()

	rb := New(server.URL, "test-key")
	_, err := rb.Transcribe(context.Background(), []byte{0x01})
	if err == nil {
		t.Error("expected error for 501 response")
	}
}

func TestSpeak_Success(t *testing.T) {
	wavData := []byte("RIFF....WAVEfmt data....")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/audio/speak" {
			t.Errorf("expected /v1/audio/speak, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Verify request body.
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.Text != "hello world" {
			t.Errorf("expected 'hello world', got %q", req.Text)
		}

		w.Header().Set("Content-Type", "audio/wav")
		w.Write(wavData)
	}))
	defer server.Close()

	rb := New(server.URL, "test-key")
	result, err := rb.Speak(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != len(wavData) {
		t.Errorf("expected %d bytes, got %d", len(wavData), len(result))
	}
}

func TestSpeak_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "piper crashed"})
	}))
	defer server.Close()

	rb := New(server.URL, "test-key")
	_, err := rb.Speak(context.Background(), "hello")
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestSpeak_NotImplemented(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
		json.NewEncoder(w).Encode(map[string]string{"error": "not configured"})
	}))
	defer server.Close()

	rb := New(server.URL, "test-key")
	_, err := rb.Speak(context.Background(), "hello")
	if err == nil {
		t.Error("expected error for 501 response")
	}
}

func TestTranscribe_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	rb := New(server.URL, "test-key")
	_, err := rb.Transcribe(context.Background(), []byte{0x01})
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}
