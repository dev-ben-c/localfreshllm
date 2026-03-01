package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// Request body size limits.
const (
	maxJSONBodySize  = 1 << 20  // 1 MB for JSON requests
	maxAudioBodySize = 50 << 20 // 50 MB for audio uploads
	maxTTSTextLen    = 10000    // 10K chars for TTS input
)

// handleTranscribe accepts raw PCM audio (16kHz, 16-bit mono) and returns transcribed text.
// POST /v1/audio/transcribe
// Content-Type: application/octet-stream
// Body: raw PCM bytes
// Response: {"text": "..."}
func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if s.whisper == nil {
		jsonError(w, http.StatusNotImplemented, "speech-to-text not configured (start server with --whisper-url)")
		return
	}

	// Content-type validation: reject if set but not application/octet-stream.
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "application/octet-stream") {
		jsonError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/octet-stream")
		return
	}

	// Enforce body size limit — returns 413 on overflow.
	r.Body = http.MaxBytesReader(w, r.Body, maxAudioBodySize)
	pcmData, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			jsonError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		jsonError(w, http.StatusBadRequest, "failed to read request body")
		log.Printf("read transcribe body: %v", err)
		return
	}

	if len(pcmData) == 0 {
		jsonError(w, http.StatusBadRequest, "empty audio data")
		return
	}

	text, err := s.whisper.Transcribe(r.Context(), pcmData)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "transcription failed")
		log.Printf("transcription failed: %v", err)
		return
	}

	jsonOK(w, map[string]string{"text": text})
}

// handleSpeak accepts a text string and returns WAV audio.
// POST /v1/audio/speak
// Content-Type: application/json
// Body: {"text": "..."}
// Response: audio/wav
func (s *Server) handleSpeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if s.piper == nil {
		jsonError(w, http.StatusNotImplemented, "text-to-speech not configured (start server with --piper-model)")
		return
	}

	// Content-type validation: reject if set but not application/json.
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "application/json") {
		jsonError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	// Enforce body size limit.
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodySize)
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			jsonError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Text == "" {
		jsonError(w, http.StatusBadRequest, "text is required")
		return
	}

	if len(req.Text) > maxTTSTextLen {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("text too long (max %d characters)", maxTTSTextLen))
		return
	}

	wavData, err := s.piper.Speak(r.Context(), req.Text)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "speech synthesis failed")
		log.Printf("speech synthesis failed: %v", err)
		return
	}

	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(wavData)))
	w.Write(wavData)
}
