package capture

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Recorder captures audio from the microphone using system audio tools.
// No CGO — uses parec (PipeWire/PulseAudio) or arecord (ALSA) as subprocesses.
type Recorder struct {
	Device   string // PulseAudio/ALSA source name (empty = system default).
	mu       sync.Mutex
	cmd      *exec.Cmd
	stdout   io.ReadCloser
	buf      bytes.Buffer
	cancel   context.CancelFunc
	running  bool
}

// recordBackend describes which tool to use for recording.
type recordBackend int

const (
	backendParec recordBackend = iota
	backendArecord
)

// detect returns the best available recording backend.
func detect() (recordBackend, error) {
	if _, err := exec.LookPath("parec"); err == nil {
		return backendParec, nil
	}
	if _, err := exec.LookPath("arecord"); err == nil {
		return backendArecord, nil
	}
	return 0, fmt.Errorf("no audio capture tool found (install pulseaudio-utils or alsa-utils)")
}

// Start begins recording audio. Call Stop() to finish and get the PCM data.
// Records 16kHz, 16-bit signed LE, mono raw PCM.
func (r *Recorder) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("already recording")
	}

	be, err := detect()
	if err != nil {
		return err
	}

	rCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	r.cmd = buildCaptureCmd(rCtx, be, r.Device)

	stdout, err := r.cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stdout pipe: %w", err)
	}
	r.stdout = stdout
	r.buf.Reset()

	if err := r.cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start recording: %w", err)
	}

	r.running = true

	// Read audio data in background.
	go func() {
		io.Copy(&r.buf, stdout)
	}()

	return nil
}

// Stop ends recording and returns the captured raw PCM data (16kHz, 16-bit mono).
func (r *Recorder) Stop() ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return nil, fmt.Errorf("not recording")
	}

	r.running = false

	// Cancel the context to kill the subprocess.
	if r.cancel != nil {
		r.cancel()
	}

	// Wait for process to exit (ignore error since we killed it).
	r.cmd.Wait()

	data := make([]byte, r.buf.Len())
	copy(data, r.buf.Bytes())
	r.buf.Reset()

	if len(data) == 0 {
		return nil, fmt.Errorf("no audio captured")
	}

	return data, nil
}

// IsRecording returns whether the recorder is currently active.
func (r *Recorder) IsRecording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// Available checks whether any audio capture tool is installed.
func Available() bool {
	_, err := detect()
	return err == nil
}

// buildCaptureCmd creates the parec/arecord command with optional device selection.
func buildCaptureCmd(ctx context.Context, be recordBackend, device string) *exec.Cmd {
	switch be {
	case backendParec:
		args := []string{
			"--format=s16le",
			"--rate=16000",
			"--channels=1",
			"--raw",
		}
		if device != "" {
			args = append(args, "--device="+device)
		}
		return exec.CommandContext(ctx, "parec", args...)
	case backendArecord:
		args := []string{
			"-f", "S16_LE",
			"-r", "16000",
			"-c", "1",
			"-t", "raw",
			"-q",
		}
		if device != "" {
			args = append(args, "-D", device)
		}
		args = append(args, "-")
		return exec.CommandContext(ctx, "arecord", args...)
	}
	return nil
}

// Source describes an available audio input device.
type Source struct {
	Name        string // PulseAudio/ALSA source name (pass to Device field).
	Description string // Human-readable description.
}

// ListSources returns available audio input sources.
func ListSources() ([]Source, error) {
	be, err := detect()
	if err != nil {
		return nil, err
	}

	switch be {
	case backendParec:
		return listPulseAudioSources()
	case backendArecord:
		return listAlsaSources()
	}
	return nil, nil
}

// listPulseAudioSources uses pactl to enumerate sources.
func listPulseAudioSources() ([]Source, error) {
	out, err := exec.Command("pactl", "list", "sources", "short").Output()
	if err != nil {
		return nil, fmt.Errorf("pactl list sources: %w", err)
	}
	var sources []Source
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			sources = append(sources, Source{
				Name:        fields[1],
				Description: strings.Join(fields[1:], " "),
			})
		}
	}
	return sources, nil
}

// listAlsaSources uses arecord -l to enumerate capture devices.
func listAlsaSources() ([]Source, error) {
	out, err := exec.Command("arecord", "-l").Output()
	if err != nil {
		return nil, fmt.Errorf("arecord -l: %w", err)
	}
	var sources []Source
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "card ") {
			sources = append(sources, Source{
				Name:        line,
				Description: line,
			})
		}
	}
	return sources, nil
}
