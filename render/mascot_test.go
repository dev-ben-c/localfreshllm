package render

import (
	"strings"
	"testing"
)

func TestRenderMascot_NotEmpty(t *testing.T) {
	result := RenderMascot(LemonIdle)
	if result == "" {
		t.Error("expected non-empty rendered mascot")
	}
}

func TestRenderMascot_PreservesContent(t *testing.T) {
	result := RenderMascot(LemonIdle)
	// Should contain the braille leaf pattern (may be wrapped in ANSI codes).
	if !strings.Contains(result, "⣠⠤⡀") {
		t.Error("rendered mascot missing leaf braille pattern")
	}
}

func TestRenderMascot_MultipleLines(t *testing.T) {
	result := RenderMascot(LemonIdle)
	lines := strings.Split(result, "\n")
	if len(lines) < 5 {
		t.Errorf("expected at least 5 lines in rendered mascot, got %d", len(lines))
	}
}

func TestLemonThinkingFrames(t *testing.T) {
	frames := LemonThinkingFrames()
	if len(frames) != 3 {
		t.Fatalf("expected 3 thinking frames, got %d", len(frames))
	}

	if frames[0] != LemonThinking1 {
		t.Error("frame 0 should be LemonThinking1")
	}
	if frames[1] != LemonThinking2 {
		t.Error("frame 1 should be LemonThinking2")
	}
	if frames[2] != LemonThinking3 {
		t.Error("frame 2 should be LemonThinking3")
	}
}

func TestLemonThinkingBackwardCompatibility(t *testing.T) {
	if LemonThinking != LemonThinking1 {
		t.Error("LemonThinking should be an alias for LemonThinking1")
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty", "", nil},
		{"single line", "hello", []string{"hello"}},
		{"two lines", "a\nb", []string{"a", "b"}},
		{"trailing newline", "a\nb\n", []string{"a", "b"}},
		{"multiple newlines", "a\n\nb\n", []string{"a", "", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("splitLines(%q): got %d lines, expected %d", tt.input, len(got), len(tt.expected))
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("splitLines(%q)[%d]: got %q, expected %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestLemonStates_NotEmpty(t *testing.T) {
	states := []struct {
		name string
		art  string
	}{
		{"Idle", LemonIdle},
		{"Thinking1", LemonThinking1},
		{"Thinking2", LemonThinking2},
		{"Thinking3", LemonThinking3},
		{"Speaking", LemonSpeaking},
	}

	for _, s := range states {
		t.Run(s.name, func(t *testing.T) {
			if s.art == "" {
				t.Error("mascot art should not be empty")
			}
			lines := splitLines(s.art)
			if len(lines) < 6 {
				t.Errorf("expected at least 6 lines, got %d", len(lines))
			}
		})
	}
}

func TestThinkingFrames_Distinct(t *testing.T) {
	frames := LemonThinkingFrames()
	for i := 0; i < len(frames); i++ {
		for j := i + 1; j < len(frames); j++ {
			if frames[i] == frames[j] {
				t.Errorf("frames %d and %d are identical", i, j)
			}
		}
	}
}
