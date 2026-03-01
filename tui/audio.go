package tui

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rabidclock/localfreshllm/audio"
	"github.com/rabidclock/localfreshllm/audio/capture"
	"github.com/rabidclock/localfreshllm/audio/playback"
	"github.com/rabidclock/localfreshllm/client"
)

// AudioState tracks voice I/O state in the TUI.
type AudioState struct {
	MicEnabled bool
	TTSEnabled bool
	Recording  bool
	Playing    bool
	recorder   *capture.Recorder
	player     *playback.Player
}

// NewAudioState creates an audio state. Mic and TTS start disabled.
func NewAudioState() AudioState {
	return AudioState{
		recorder: &capture.Recorder{},
		player:   &playback.Player{},
	}
}

// Tea messages for audio events.
type audioRecordDoneMsg struct {
	pcm []byte
	err error
}

type audioTranscribeDoneMsg struct {
	text string
	err  error
}

type audioPlayDoneMsg struct {
	err error
}

// startRecording begins mic capture. Returns a command that blocks until stopped externally.
func (a *AudioState) startRecording(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		if err := a.recorder.Start(ctx); err != nil {
			return audioRecordDoneMsg{err: err}
		}
		// Recording continues until stopRecording is called.
		// This command doesn't block — the Stop() call produces the result.
		return nil
	}
}

// stopRecording stops mic capture and returns the PCM data.
func (a *AudioState) stopRecording() tea.Cmd {
	return func() tea.Msg {
		pcm, err := a.recorder.Stop()
		return audioRecordDoneMsg{pcm: pcm, err: err}
	}
}

// transcribeAudio sends PCM data to the server for STT.
func transcribeAudio(b interface{}, pcm []byte) tea.Cmd {
	return func() tea.Msg {
		remote, ok := b.(*client.RemoteBackend)
		if !ok {
			return audioTranscribeDoneMsg{err: nil}
		}
		text, err := remote.Transcribe(context.Background(), pcm)
		return audioTranscribeDoneMsg{text: text, err: err}
	}
}

// playTTS synthesizes speech and plays it. Uses local piper if configured,
// otherwise falls back to the remote server's /v1/audio/speak endpoint.
func playTTS(cfg Config, p *playback.Player, text string) tea.Cmd {
	return func() tea.Msg {
		clean := sanitizeForTTS(text)
		if clean == "" {
			return audioPlayDoneMsg{}
		}

		var wavData []byte
		var err error

		if cfg.PiperModel != "" {
			piper := audio.NewPiperTTS(cfg.PiperModel)
			wavData, err = piper.Speak(context.Background(), clean)
		} else if remote, ok := cfg.Backend.(*client.RemoteBackend); ok {
			wavData, err = remote.Speak(context.Background(), clean)
		} else {
			return audioPlayDoneMsg{}
		}

		if err != nil {
			return audioPlayDoneMsg{err: err}
		}
		err = p.Play(context.Background(), wavData)
		return audioPlayDoneMsg{err: err}
	}
}

var (
	// Markdown code blocks (fenced and inline).
	reCodeBlock = regexp.MustCompile("(?s)```.*?```")
	reInlineCode = regexp.MustCompile("`[^`]+`")
	// Markdown headers.
	reHeader = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	// Markdown bold/italic markers.
	reBoldItalic = regexp.MustCompile(`\*{1,3}`)
	// URLs.
	reURL = regexp.MustCompile(`https?://\S+`)
	// Bullet list markers.
	reBullet = regexp.MustCompile(`(?m)^\s*[-*+]\s+`)
	// Numbered list markers.
	reNumbered = regexp.MustCompile(`(?m)^\s*\d+\.\s+`)
	// Multiple whitespace/newlines.
	reWhitespace = regexp.MustCompile(`\s+`)
)

// sanitizeForTTS cleans LLM output for natural-sounding speech.
func sanitizeForTTS(text string) string {
	// Remove code blocks entirely — they sound terrible spoken.
	text = reCodeBlock.ReplaceAllString(text, " ")
	text = reInlineCode.ReplaceAllString(text, " ")

	// Remove URLs.
	text = reURL.ReplaceAllString(text, " ")

	// Remove markdown formatting.
	text = reHeader.ReplaceAllString(text, "")
	text = reBoldItalic.ReplaceAllString(text, "")
	text = reBullet.ReplaceAllString(text, "")
	text = reNumbered.ReplaceAllString(text, "")

	// Remove characters that get read literally.
	text = strings.NewReplacer(
		"(", "", ")", "",
		"[", "", "]", "",
		"{", "", "}", "",
		"~", "", "_", " ",
		"|", "", ">", "",
		"#", "", "```", "",
	).Replace(text)

	// Strip emojis and other non-speech unicode.
	var b strings.Builder
	for _, r := range text {
		if isSpokenRune(r) {
			b.WriteRune(r)
		}
	}
	text = b.String()

	// Collapse whitespace.
	text = reWhitespace.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// isSpokenRune returns true for characters that make sense in spoken text.
func isSpokenRune(r rune) bool {
	if r <= 127 {
		return true // ASCII
	}
	// Allow common Latin/extended characters and CJK.
	if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsPunct(r) {
		// Reject emoji-range punctuation/symbols.
		if r >= 0x2600 {
			return false
		}
		return true
	}
	if unicode.IsSpace(r) {
		return true
	}
	return false
}

// HandleAudioKey processes Ctrl+Space toggle for push-to-talk.
// Returns true if the key was handled, plus any commands to execute.
func (a *AudioState) HandleAudioKey(msg tea.KeyMsg, cfg *Config) (bool, tea.Cmd) {
	// Ctrl+Space or F5 toggles recording.
	isToggle := (msg.Type == tea.KeyCtrlAt) || // Ctrl+Space on some terminals
		(msg.Type == tea.KeyF5)

	if !isToggle || !a.MicEnabled {
		return false, nil
	}

	if a.Recording {
		a.Recording = false
		return true, a.stopRecording()
	}

	a.Recording = true
	return true, a.startRecording(context.Background())
}

// AudioAvailable returns whether mic capture and playback tools are installed.
func AudioAvailable() (mic bool, speaker bool) {
	return capture.Available(), playback.Available()
}
