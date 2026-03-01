package capture

import (
	"strings"
	"testing"
)

func TestValidateDevice_Valid(t *testing.T) {
	valid := []string{
		"",                    // empty = system default
		"default",             // simple name
		"hw:0,0",              // ALSA device
		"hw:1,0",              // ALSA device
		"alsa_input.usb-Audio_Device-00.analog-stereo", // PulseAudio source
		"pulse-source.monitor",
		"my-device_123",
		"a",
		strings.Repeat("a", 256), // max length
	}

	for _, name := range valid {
		if err := ValidateDevice(name); err != nil {
			t.Errorf("ValidateDevice(%q) unexpected error: %v", name, err)
		}
	}
}

func TestValidateDevice_Invalid(t *testing.T) {
	invalid := []string{
		"device; rm -rf /",           // command injection
		"$(echo pwned)",              // command substitution
		"device`whoami`",             // backtick injection
		"device name with spaces",    // spaces
		"device\nnewline",            // newline
		"device\ttab",                // tab
		"device/path",                // path separator
		"device|pipe",                // pipe
		"device&background",          // background
		"device>redirect",            // redirect
		"device<input",               // input redirect
		strings.Repeat("a", 257),     // too long
	}

	for _, name := range invalid {
		if err := ValidateDevice(name); err == nil {
			t.Errorf("ValidateDevice(%q) expected error, got nil", name)
		}
	}
}
