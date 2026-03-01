package audio

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// PiperTTS runs Piper TTS as a subprocess to convert text to speech.
type PiperTTS struct {
	ModelPath string
}

// NewPiperTTS creates a TTS instance using the given Piper model.
func NewPiperTTS(modelPath string) *PiperTTS {
	return &PiperTTS{ModelPath: modelPath}
}

// Speak converts text to WAV audio using Piper.
// Returns raw WAV data (22050 Hz, 16-bit mono).
func (p *PiperTTS) Speak(ctx context.Context, text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("empty text")
	}

	cmd := exec.CommandContext(ctx, "piper",
		"--model", p.ModelPath,
		"--output_file", "-",
		"--quiet",
	)

	cmd.Stdin = strings.NewReader(text)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("piper failed: %s: %w", errMsg, err)
		}
		return nil, fmt.Errorf("piper failed: %w", err)
	}

	if stdout.Len() == 0 {
		return nil, fmt.Errorf("piper produced no output")
	}

	return stdout.Bytes(), nil
}

// Available checks whether the piper binary is installed.
func (p *PiperTTS) Available() bool {
	_, err := exec.LookPath("piper")
	return err == nil
}
