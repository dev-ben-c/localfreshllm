package capture

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os/exec"
	"sync"
)

// VAD parameters.
const (
	sampleRate    = 16000
	chunkMs       = 100
	chunkSamples  = sampleRate * chunkMs / 1000 // 1600
	chunkBytes    = chunkSamples * 2             // 3200 (16-bit)
	silenceChunks = 15                           // 1.5s of silence to end segment
	minSpeechMs   = 500                          // minimum speech duration
	minChunks     = minSpeechMs / chunkMs        // 5 chunks
	leadInChunks  = 3                            // 300ms pre-speech buffer
	rmsThreshold  = 400                          // RMS threshold for speech
)

// Listener continuously records audio and emits speech segments
// using energy-based voice activity detection.
type Listener struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdout  io.ReadCloser
	cancel  context.CancelFunc
	running bool
}

// Start begins continuous audio capture. Call NextSegment() to get speech segments.
func (l *Listener) Start(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.running {
		return fmt.Errorf("listener already running")
	}

	be, err := detect()
	if err != nil {
		return err
	}

	lCtx, cancel := context.WithCancel(ctx)
	l.cancel = cancel

	switch be {
	case backendParec:
		l.cmd = exec.CommandContext(lCtx, "parec",
			"--format=s16le",
			"--rate=16000",
			"--channels=1",
			"--raw",
		)
	case backendArecord:
		l.cmd = exec.CommandContext(lCtx, "arecord",
			"-f", "S16_LE",
			"-r", "16000",
			"-c", "1",
			"-t", "raw",
			"-q",
			"-",
		)
	}

	stdout, err := l.cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stdout pipe: %w", err)
	}
	l.stdout = stdout

	if err := l.cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start listener: %w", err)
	}

	l.running = true
	return nil
}

// NextSegment blocks until a speech segment is detected, then returns the raw PCM.
// Returns an error if the listener is stopped or the context is cancelled.
func (l *Listener) NextSegment() ([]byte, error) {
	l.mu.Lock()
	if !l.running {
		l.mu.Unlock()
		return nil, fmt.Errorf("listener not running")
	}
	stdout := l.stdout
	l.mu.Unlock()

	// Ring buffer for lead-in (captures audio just before speech starts).
	leadIn := make([][]byte, leadInChunks)
	leadIdx := 0

	var segment []byte
	silenceCount := 0
	speechCount := 0
	speaking := false

	chunk := make([]byte, chunkBytes)
	for {
		_, err := io.ReadFull(stdout, chunk)
		if err != nil {
			return nil, fmt.Errorf("read audio: %w", err)
		}

		rms := chunkRMS(chunk)
		isSpeech := rms > rmsThreshold

		if !speaking {
			// Store in lead-in ring buffer.
			buf := make([]byte, chunkBytes)
			copy(buf, chunk)
			leadIn[leadIdx%leadInChunks] = buf
			leadIdx++

			if isSpeech {
				speaking = true
				speechCount = 1
				silenceCount = 0

				// Prepend lead-in buffer.
				for i := 0; i < leadInChunks; i++ {
					idx := (leadIdx - leadInChunks + i) % leadInChunks
					if idx < 0 {
						idx += leadInChunks
					}
					if leadIn[idx] != nil {
						segment = append(segment, leadIn[idx]...)
					}
				}
				segment = append(segment, chunk...)
			}
		} else {
			segment = append(segment, chunk...)

			if isSpeech {
				speechCount++
				silenceCount = 0
			} else {
				silenceCount++
				if silenceCount >= silenceChunks {
					// End of speech detected.
					if speechCount >= minChunks {
						return segment, nil
					}
					// Too short — was probably a click or noise. Reset.
					segment = nil
					speaking = false
					speechCount = 0
					silenceCount = 0
				}
			}
		}
	}
}

// Stop ends the continuous listener.
func (l *Listener) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.running {
		return
	}
	l.running = false
	if l.cancel != nil {
		l.cancel()
	}
	l.cmd.Wait()
}

// IsRunning returns whether the listener is active.
func (l *Listener) IsRunning() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.running
}

// chunkRMS calculates the root mean square energy of a 16-bit PCM chunk.
func chunkRMS(data []byte) float64 {
	if len(data) < 2 {
		return 0
	}
	var sum float64
	n := len(data) / 2
	for i := 0; i < n; i++ {
		sample := int16(binary.LittleEndian.Uint16(data[i*2:]))
		sum += float64(sample) * float64(sample)
	}
	return math.Sqrt(sum / float64(n))
}
