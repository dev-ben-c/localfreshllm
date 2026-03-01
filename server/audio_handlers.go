package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// handleTranscribe accepts raw PCM audio (16kHz, 16-bit mono) and returns transcribed text.
// POST /v1/audio/transcribe
// Content-Type: application/octet-stream
// Body: raw PCM bytes
// Response: {"text": "..."}
func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if s.whisper == nil {
		http.Error(w, `{"error":"speech-to-text not configured (start server with --whisper-url)"}`, http.StatusNotImplemented)
		return
	}

	pcmData, err := io.ReadAll(io.LimitReader(r.Body, 50*1024*1024)) // 50 MB max
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"read body: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if len(pcmData) == 0 {
		http.Error(w, `{"error":"empty audio data"}`, http.StatusBadRequest)
		return
	}

	text, err := s.whisper.Transcribe(r.Context(), pcmData)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"transcription failed: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	resp, _ := json.Marshal(map[string]string{"text": text})
	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

// handleSpeak accepts a text string and returns WAV audio.
// POST /v1/audio/speak
// Content-Type: application/json
// Body: {"text": "..."}
// Response: audio/wav
func (s *Server) handleSpeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if s.piper == nil {
		http.Error(w, `{"error":"text-to-speech not configured (start server with --piper-model)"}`, http.StatusNotImplemented)
		return
	}

	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		http.Error(w, `{"error":"text is required"}`, http.StatusBadRequest)
		return
	}

	wavData, err := s.piper.Speak(r.Context(), req.Text)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"speech synthesis failed: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(wavData)))
	w.Write(wavData)
}
