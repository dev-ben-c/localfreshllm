package audio

import (
	"encoding/binary"
	"testing"
)

func TestWriteWAVHeader_BasicStructure(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0x03, 0x04}
	wav := WriteWAVHeader(pcm, 16000)

	if len(wav) != wavHeaderSize+len(pcm) {
		t.Fatalf("expected length %d, got %d", wavHeaderSize+len(pcm), len(wav))
	}

	// RIFF header.
	if string(wav[0:4]) != "RIFF" {
		t.Errorf("expected RIFF magic, got %q", string(wav[0:4]))
	}
	if string(wav[8:12]) != "WAVE" {
		t.Errorf("expected WAVE format, got %q", string(wav[8:12]))
	}

	// File size field = total - 8.
	fileSize := binary.LittleEndian.Uint32(wav[4:8])
	if fileSize != uint32(len(wav)-8) {
		t.Errorf("RIFF size: expected %d, got %d", len(wav)-8, fileSize)
	}

	// fmt sub-chunk.
	if string(wav[12:16]) != "fmt " {
		t.Errorf("expected fmt sub-chunk, got %q", string(wav[12:16]))
	}
	fmtSize := binary.LittleEndian.Uint32(wav[16:20])
	if fmtSize != 16 {
		t.Errorf("fmt chunk size: expected 16, got %d", fmtSize)
	}
	audioFormat := binary.LittleEndian.Uint16(wav[20:22])
	if audioFormat != 1 {
		t.Errorf("audio format: expected 1 (PCM), got %d", audioFormat)
	}
	channels := binary.LittleEndian.Uint16(wav[22:24])
	if channels != 1 {
		t.Errorf("channels: expected 1, got %d", channels)
	}
	sampleRate := binary.LittleEndian.Uint32(wav[24:28])
	if sampleRate != 16000 {
		t.Errorf("sample rate: expected 16000, got %d", sampleRate)
	}
	bitsPerSample := binary.LittleEndian.Uint16(wav[34:36])
	if bitsPerSample != 16 {
		t.Errorf("bits per sample: expected 16, got %d", bitsPerSample)
	}

	// Byte rate = sampleRate * channels * bitsPerSample/8.
	byteRate := binary.LittleEndian.Uint32(wav[28:32])
	expectedByteRate := uint32(16000 * 1 * 2)
	if byteRate != expectedByteRate {
		t.Errorf("byte rate: expected %d, got %d", expectedByteRate, byteRate)
	}

	// Block align.
	blockAlign := binary.LittleEndian.Uint16(wav[32:34])
	if blockAlign != 2 {
		t.Errorf("block align: expected 2, got %d", blockAlign)
	}

	// data sub-chunk.
	if string(wav[36:40]) != "data" {
		t.Errorf("expected data sub-chunk, got %q", string(wav[36:40]))
	}
	dataSize := binary.LittleEndian.Uint32(wav[40:44])
	if dataSize != uint32(len(pcm)) {
		t.Errorf("data size: expected %d, got %d", len(pcm), dataSize)
	}

	// PCM data preserved.
	for i, b := range pcm {
		if wav[wavHeaderSize+i] != b {
			t.Errorf("pcm byte %d: expected 0x%02x, got 0x%02x", i, b, wav[wavHeaderSize+i])
		}
	}
}

func TestWriteWAVHeader_EmptyPCM(t *testing.T) {
	wav := WriteWAVHeader(nil, 44100)

	if len(wav) != wavHeaderSize {
		t.Fatalf("expected length %d for empty PCM, got %d", wavHeaderSize, len(wav))
	}

	dataSize := binary.LittleEndian.Uint32(wav[40:44])
	if dataSize != 0 {
		t.Errorf("data size: expected 0, got %d", dataSize)
	}
}

func TestWriteWAVHeader_DifferentSampleRates(t *testing.T) {
	rates := []int{8000, 16000, 22050, 44100, 48000}
	pcm := make([]byte, 100)

	for _, rate := range rates {
		wav := WriteWAVHeader(pcm, rate)
		gotRate := binary.LittleEndian.Uint32(wav[24:28])
		if gotRate != uint32(rate) {
			t.Errorf("sample rate %d: got %d in header", rate, gotRate)
		}
	}
}

func TestParseWAVHeader_RoundTrip(t *testing.T) {
	original := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0x11, 0x22, 0x33, 0x44}
	wav := WriteWAVHeader(original, 16000)

	sampleRate, pcm, err := ParseWAVHeader(wav)
	if err != nil {
		t.Fatalf("ParseWAVHeader failed: %v", err)
	}

	if sampleRate != 16000 {
		t.Errorf("sample rate: expected 16000, got %d", sampleRate)
	}

	if len(pcm) != len(original) {
		t.Fatalf("PCM length: expected %d, got %d", len(original), len(pcm))
	}

	for i := range original {
		if pcm[i] != original[i] {
			t.Errorf("PCM byte %d: expected 0x%02x, got 0x%02x", i, original[i], pcm[i])
		}
	}
}

func TestParseWAVHeader_RoundTripMultipleRates(t *testing.T) {
	pcm := make([]byte, 256)
	for i := range pcm {
		pcm[i] = byte(i)
	}

	for _, rate := range []int{8000, 22050, 44100, 48000} {
		wav := WriteWAVHeader(pcm, rate)
		gotRate, gotPCM, err := ParseWAVHeader(wav)
		if err != nil {
			t.Errorf("rate %d: parse failed: %v", rate, err)
			continue
		}
		if gotRate != rate {
			t.Errorf("rate %d: got %d", rate, gotRate)
		}
		if len(gotPCM) != len(pcm) {
			t.Errorf("rate %d: PCM length %d, expected %d", rate, len(gotPCM), len(pcm))
		}
	}
}

func TestParseWAVHeader_EmptyData(t *testing.T) {
	wav := WriteWAVHeader(nil, 16000)
	rate, pcm, err := ParseWAVHeader(wav)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 16000 {
		t.Errorf("sample rate: expected 16000, got %d", rate)
	}
	if len(pcm) != 0 {
		t.Errorf("expected empty PCM, got %d bytes", len(pcm))
	}
}

func TestParseWAVHeader_TooShort(t *testing.T) {
	_, _, err := ParseWAVHeader([]byte{0x01, 0x02})
	if err == nil {
		t.Error("expected error for short data")
	}
}

func TestParseWAVHeader_InvalidMagic(t *testing.T) {
	data := make([]byte, 48)
	copy(data[0:4], "NOTW")
	copy(data[8:12], "WAVE")

	_, _, err := ParseWAVHeader(data)
	if err == nil {
		t.Error("expected error for invalid RIFF magic")
	}
}

func TestParseWAVHeader_InvalidFormat(t *testing.T) {
	data := make([]byte, 48)
	copy(data[0:4], "RIFF")
	copy(data[8:12], "NOPE")

	_, _, err := ParseWAVHeader(data)
	if err == nil {
		t.Error("expected error for invalid WAVE format")
	}
}

func TestParseWAVHeader_NoDataChunk(t *testing.T) {
	// Valid RIFF/WAVE header but with a non-data chunk and no data chunk.
	data := make([]byte, 56)
	copy(data[0:4], "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	copy(data[8:12], "WAVE")

	// fmt chunk.
	copy(data[12:16], "fmt ")
	binary.LittleEndian.PutUint32(data[16:20], 16)
	binary.LittleEndian.PutUint16(data[20:22], 1)
	binary.LittleEndian.PutUint16(data[22:24], 1)
	binary.LittleEndian.PutUint32(data[24:28], 16000)
	binary.LittleEndian.PutUint32(data[28:32], 32000)
	binary.LittleEndian.PutUint16(data[32:34], 2)
	binary.LittleEndian.PutUint16(data[34:36], 16)

	// A non-data chunk instead of data.
	copy(data[36:40], "LIST")
	binary.LittleEndian.PutUint32(data[40:44], 12) // 12 bytes of LIST data
	// 12 bytes of padding fill...

	_, _, err := ParseWAVHeader(data)
	if err == nil {
		t.Error("expected error for missing data chunk")
	}
}

func TestParseWAVHeader_TruncatedPCM(t *testing.T) {
	// Create a WAV that claims 100 bytes of data but only has 50.
	pcm := make([]byte, 50)
	wav := WriteWAVHeader(pcm, 16000)

	// Overwrite the data size to claim 100 bytes.
	binary.LittleEndian.PutUint32(wav[40:44], 100)

	rate, gotPCM, err := ParseWAVHeader(wav)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 16000 {
		t.Errorf("sample rate: expected 16000, got %d", rate)
	}
	// Should clamp to actual available data.
	if len(gotPCM) != 50 {
		t.Errorf("expected 50 bytes (clamped), got %d", len(gotPCM))
	}
}
