package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rabidclock/localfreshllm/audio/capture"
)

const wakeWord = "lemon"

// voiceSegmentMsg carries a detected speech segment from the listener.
type voiceSegmentMsg struct {
	pcm []byte
	err error
}

// voiceTranscribedMsg carries the transcription of a voice segment.
type voiceTranscribedMsg struct {
	text string
	err  error
}

// startAndListen launches the listener and immediately waits for the first segment.
func startAndListen(listener *capture.Listener) tea.Cmd {
	return func() tea.Msg {
		if err := listener.Start(context.Background()); err != nil {
			return voiceSegmentMsg{err: err}
		}
		pcm, err := listener.NextSegment()
		return voiceSegmentMsg{pcm: pcm, err: err}
	}
}

// listenForSegment blocks until the next speech segment is detected.
func listenForSegment(listener *capture.Listener) tea.Cmd {
	return func() tea.Msg {
		pcm, err := listener.NextSegment()
		return voiceSegmentMsg{pcm: pcm, err: err}
	}
}

// transcribeVoiceSegment sends a voice segment to Whisper and returns the result
// as a voiceTranscribedMsg (separate from audioTranscribeDoneMsg to keep flows distinct).
func transcribeVoiceSegment(cfg Config, pcm []byte) tea.Cmd {
	return func() tea.Msg {
		// Reuse the same transcription logic but return a different message type.
		result := transcribeAudio(cfg, pcm)()
		if msg, ok := result.(audioTranscribeDoneMsg); ok {
			return voiceTranscribedMsg{text: msg.text, err: msg.err}
		}
		return voiceTranscribedMsg{}
	}
}

// extractAfterWakeWord checks if the text starts with the wake word
// and returns the remaining text. Case-insensitive.
func extractAfterWakeWord(text string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))

	// Check for "lemon" or "lemon," at the start.
	prefixes := []string{
		wakeWord + " ",
		wakeWord + ", ",
		wakeWord + ". ",
		wakeWord + "! ",
		wakeWord + "? ",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(text[len(prefix):]), true
		}
	}

	// Check if it's just the wake word by itself (with optional punctuation).
	trimmed := strings.TrimRight(lower, ".,!? ")
	if trimmed == wakeWord {
		return "", true
	}

	return "", false
}
