package playback

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/rabidclock/localfreshllm/audio"
)

// Player plays audio using system audio tools.
// No CGO — uses paplay (PipeWire/PulseAudio) or aplay (ALSA) as subprocesses.
type Player struct{}

// playbackBackend describes which tool to use for playback.
type playbackBackend int

const (
	backendPaplay playbackBackend = iota
	backendAplay
)

// detect returns the best available playback backend.
func detect() (playbackBackend, error) {
	if _, err := exec.LookPath("paplay"); err == nil {
		return backendPaplay, nil
	}
	if _, err := exec.LookPath("aplay"); err == nil {
		return backendAplay, nil
	}
	return 0, fmt.Errorf("no audio playback tool found (install pulseaudio-utils or alsa-utils)")
}

// Play plays WAV audio data through the system speakers.
func (p *Player) Play(ctx context.Context, wavData []byte) error {
	be, err := detect()
	if err != nil {
		return err
	}

	// Parse WAV to get sample rate and raw PCM.
	sampleRate, pcm, err := audio.ParseWAVHeader(wavData)
	if err != nil {
		return fmt.Errorf("parse wav: %w", err)
	}

	var cmd *exec.Cmd
	switch be {
	case backendPaplay:
		// paplay can play raw PCM with format flags.
		cmd = exec.CommandContext(ctx, "paplay",
			"--format=s16le",
			fmt.Sprintf("--rate=%d", sampleRate),
			"--channels=1",
			"--raw",
		)
		cmd.Stdin = bytes.NewReader(pcm)
	case backendAplay:
		cmd = exec.CommandContext(ctx, "aplay",
			"-f", "S16_LE",
			"-r", fmt.Sprintf("%d", sampleRate),
			"-c", "1",
			"-t", "raw",
			"-q",
			"-",
		)
		cmd.Stdin = bytes.NewReader(pcm)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("playback failed: %s: %w", errMsg, err)
		}
		return fmt.Errorf("playback failed: %w", err)
	}

	return nil
}

// Available checks whether any audio playback tool is installed.
func Available() bool {
	_, err := detect()
	return err == nil
}
