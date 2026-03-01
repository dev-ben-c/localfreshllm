package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rabidclock/localfreshllm/audio/capture"
)

const wakeWord = "cedric"

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

// Greeting prefixes that can precede the wake word (e.g. "hey cedric", "okay cedric").
var wakePrefixes = []string{
	"hey", "hey,",
	"okay", "okay,",
	"ok", "ok,",
	"yo", "yo,",
	"hi", "hi,",
	"hello", "hello,",
}

// extractAfterWakeWord checks if the text starts with the wake word
// (optionally preceded by a greeting) and returns the remaining text.
// Case-insensitive. Matches: "cedric ...", "hey cedric ...", "okay, cedric ...", etc.
func extractAfterWakeWord(text string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))

	// Try with greeting prefix first (e.g. "hey cedric, what time is it").
	for _, gp := range wakePrefixes {
		full := gp + " " + wakeWord
		if afterWake, ok := matchWakePrefix(lower, text, full); ok {
			return afterWake, true
		}
	}

	// Try bare wake word (e.g. "cedric, what time is it").
	if afterWake, ok := matchWakePrefix(lower, text, wakeWord); ok {
		return afterWake, true
	}

	return "", false
}

// matchWakePrefix checks if lower starts with prefix followed by punctuation/space
// or is exactly the prefix (with optional trailing punctuation).
// Returns the remaining original-case text after the match.
func matchWakePrefix(lower, original string, prefix string) (string, bool) {
	// "prefix " / "prefix, " / "prefix. " etc.
	separators := []string{" ", ", ", ". ", "! ", "? "}
	for _, sep := range separators {
		full := prefix + sep
		if strings.HasPrefix(lower, full) {
			return strings.TrimSpace(original[len(full):]), true
		}
	}

	// Exact match with optional trailing punctuation (wake word alone).
	trimmed := strings.TrimRight(lower, ".,!? ")
	if trimmed == prefix {
		return "", true
	}

	return "", false
}
