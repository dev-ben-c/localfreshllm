package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

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

// playTTS requests TTS from the server and plays the resulting audio.
func playTTS(b interface{}, p *playback.Player, text string) tea.Cmd {
	return func() tea.Msg {
		remote, ok := b.(*client.RemoteBackend)
		if !ok {
			return audioPlayDoneMsg{}
		}
		wavData, err := remote.Speak(context.Background(), text)
		if err != nil {
			return audioPlayDoneMsg{err: err}
		}
		err = p.Play(context.Background(), wavData)
		return audioPlayDoneMsg{err: err}
	}
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
