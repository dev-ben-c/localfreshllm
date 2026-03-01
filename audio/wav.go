package audio

import (
	"encoding/binary"
	"fmt"
)

// WAV header constants.
const (
	wavHeaderSize  = 44
	wavFormatPCM   = 1
	wavChannelMono = 1
	wavBitsPerSample16 = 16
)

// WriteWAVHeader prepends a WAV header to raw PCM data.
// Expects mono, 16-bit signed little-endian samples.
func WriteWAVHeader(pcm []byte, sampleRate int) []byte {
	dataLen := len(pcm)
	fileLen := dataLen + wavHeaderSize - 8

	buf := make([]byte, wavHeaderSize+dataLen)

	// RIFF header.
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(fileLen))
	copy(buf[8:12], "WAVE")

	// fmt sub-chunk.
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16) // sub-chunk size
	binary.LittleEndian.PutUint16(buf[20:22], wavFormatPCM)
	binary.LittleEndian.PutUint16(buf[22:24], wavChannelMono)
	binary.LittleEndian.PutUint32(buf[24:28], uint32(sampleRate))
	byteRate := sampleRate * wavChannelMono * (wavBitsPerSample16 / 8)
	binary.LittleEndian.PutUint32(buf[28:32], uint32(byteRate))
	blockAlign := wavChannelMono * (wavBitsPerSample16 / 8)
	binary.LittleEndian.PutUint16(buf[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(buf[34:36], wavBitsPerSample16)

	// data sub-chunk.
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataLen))

	copy(buf[wavHeaderSize:], pcm)
	return buf
}

// ParseWAVHeader reads a WAV header and returns the sample rate and raw PCM data.
func ParseWAVHeader(data []byte) (sampleRate int, pcm []byte, err error) {
	if len(data) < wavHeaderSize {
		return 0, nil, fmt.Errorf("data too short for WAV header")
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return 0, nil, fmt.Errorf("not a valid WAV file")
	}

	sampleRate = int(binary.LittleEndian.Uint32(data[24:28]))

	// Find the data sub-chunk (may not be at offset 36 if extra chunks exist).
	offset := 12
	for offset+8 <= len(data) {
		chunkID := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		if chunkID == "data" {
			start := offset + 8
			end := start + chunkSize
			if end > len(data) {
				end = len(data)
			}
			return sampleRate, data[start:end], nil
		}
		offset += 8 + chunkSize
		// Align to 2-byte boundary.
		if offset%2 != 0 {
			offset++
		}
	}

	return 0, nil, fmt.Errorf("no data chunk found in WAV")
}
