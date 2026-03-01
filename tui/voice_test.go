package tui

import "testing"

func TestExtractAfterWakeWord(t *testing.T) {
	tests := []struct {
		input    string
		wantText string
		wantOK   bool
	}{
		// Bare wake word.
		{"Cedric", "", true},
		{"cedric", "", true},
		{"CEDRIC", "", true},
		{"cedric.", "", true},
		{"cedric!", "", true},
		{"cedric?", "", true},

		// Wake word + message.
		{"Cedric what time is it", "what time is it", true},
		{"cedric, what time is it", "what time is it", true},
		{"Cedric. Tell me a joke", "Tell me a joke", true},
		{"cedric! help me", "help me", true},
		{"cedric? are you there", "are you there", true},

		// Greeting prefixes.
		{"hey cedric", "", true},
		{"Hey Cedric, what's the weather", "what's the weather", true},
		{"hey, cedric how are you", "how are you", true},
		{"okay cedric", "", true},
		{"Okay Cedric, set a timer", "set a timer", true},
		{"okay, cedric tell me a joke", "tell me a joke", true},
		{"ok cedric", "", true},
		{"OK Cedric, play some music", "play some music", true},
		{"ok, cedric what's up", "what's up", true},
		{"yo cedric", "", true},
		{"Yo Cedric, search for something", "search for something", true},
		{"yo, cedric help", "help", true},
		{"hi cedric", "", true},
		{"Hi Cedric, good morning", "good morning", true},
		{"hi, cedric what's new", "what's new", true},
		{"hello cedric", "", true},
		{"Hello Cedric, how are you", "how are you", true},
		{"hello, cedric read the news", "read the news", true},

		// Non-matches.
		{"what time is it", "", false},
		{"hey there", "", false},
		{"", "", false},
		{"cedrics cousin", "", false},
		{"hey cedrics", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			text, ok := extractAfterWakeWord(tt.input)
			if ok != tt.wantOK {
				t.Errorf("extractAfterWakeWord(%q): ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if text != tt.wantText {
				t.Errorf("extractAfterWakeWord(%q): text = %q, want %q", tt.input, text, tt.wantText)
			}
		})
	}
}
