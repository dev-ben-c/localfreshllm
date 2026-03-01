package capture

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// Recorder captures audio from the microphone using system audio tools.
// No CGO — uses parec (PipeWire/PulseAudio) or arecord (ALSA) as subprocesses.
type Recorder struct {
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

	switch be {
	case backendParec:
		r.cmd = exec.CommandContext(rCtx, "parec",
			"--format=s16le",
			"--rate=16000",
			"--channels=1",
			"--raw",
		)
	case backendArecord:
		r.cmd = exec.CommandContext(rCtx, "arecord",
			"-f", "S16_LE",
			"-r", "16000",
			"-c", "1",
			"-t", "raw",
			"-q",
			"-",
		)
	}

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
