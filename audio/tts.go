package audio

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	piperBin := "piper"
	piperDir := filepath.Dir(p.ModelPath)

	// If piper isn't on PATH, check common install locations.
	if _, err := exec.LookPath(piperBin); err != nil {
		for _, candidate := range []string{
			filepath.Join(piperDir, "..", "piper"),
			"/opt/piper/piper",
		} {
			if _, err := os.Stat(candidate); err == nil {
				piperBin = candidate
				piperDir = filepath.Dir(candidate)
				break
			}
		}
	}

	cmd := exec.CommandContext(ctx, piperBin,
		"--model", p.ModelPath,
		"--output_file", "-",
		"--quiet",
	)

	// Piper's bundled libs (espeak-ng, onnxruntime) live next to the binary.
	cmd.Env = append(os.Environ(), "LD_LIBRARY_PATH="+piperDir)
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

// Available checks whether the piper binary can be found.
func (p *PiperTTS) Available() bool {
	if _, err := exec.LookPath("piper"); err == nil {
		return true
	}
	for _, candidate := range []string{
		filepath.Join(filepath.Dir(p.ModelPath), "..", "piper"),
		"/opt/piper/piper",
	} {
		if _, err := os.Stat(candidate); err == nil {
			return true
		}
	}
	return false
}
