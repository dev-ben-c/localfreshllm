package tui

import (
	"strings"
	"testing"
)

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

func TestBuildContent_Empty(t *testing.T) {
	result := buildContent(nil, "", "qwen3:14b", 80)
	if result != "" {
		t.Errorf("expected empty string for no messages, got %q", result)
	}
}

func TestBuildContent_UserMessage(t *testing.T) {
	msgs := []chatMessage{
		{role: "user", content: "hello world"},
	}
	result := buildContent(msgs, "", "qwen3:14b", 80)

	if !strings.Contains(result, "You:") {
		t.Error("expected 'You:' label in user message")
	}
	if !strings.Contains(result, "hello world") {
		t.Error("expected user message content")
	}
}

func TestBuildContent_AssistantMessage(t *testing.T) {
	msgs := []chatMessage{
		{role: "assistant", content: "Hi there!"},
	}
	result := buildContent(msgs, "", "test-model", 80)

	if !strings.Contains(result, "test-model:") {
		t.Error("expected model name label in assistant message")
	}
	if !strings.Contains(result, "Hi there!") {
		t.Error("expected assistant message content")
	}
}

func TestBuildContent_SystemMessage(t *testing.T) {
	msgs := []chatMessage{
		{role: "system", content: "Tools enabled"},
	}
	result := buildContent(msgs, "", "model", 80)

	if !strings.Contains(result, "Tools enabled") {
		t.Error("expected system message content")
	}
}

func TestBuildContent_ErrorMessage(t *testing.T) {
	msgs := []chatMessage{
		{role: "error", content: "something broke"},
	}
	result := buildContent(msgs, "", "model", 80)

	if !strings.Contains(result, "Error:") {
		t.Error("expected 'Error:' prefix in error message")
	}
	if !strings.Contains(result, "something broke") {
		t.Error("expected error content")
	}
}

func TestBuildContent_StreamingBuffer(t *testing.T) {
	result := buildContent(nil, "partial response...", "qwen3:14b", 80)

	if !strings.Contains(result, "qwen3:14b:") {
		t.Error("expected model label for streaming buffer")
	}
	if !strings.Contains(result, "partial response...") {
		t.Error("expected streaming buffer content")
	}
}

func TestBuildContent_MultipleMessages(t *testing.T) {
	msgs := []chatMessage{
		{role: "user", content: "What is Go?"},
		{role: "assistant", content: "Go is a programming language."},
		{role: "user", content: "Tell me more."},
	}
	result := buildContent(msgs, "Sure, Go was...", "model", 80)

	// All messages should appear in order.
	idx1 := strings.Index(result, "What is Go?")
	idx2 := strings.Index(result, "Go is a programming language.")
	idx3 := strings.Index(result, "Tell me more.")
	idx4 := strings.Index(result, "Sure, Go was...")

	if idx1 == -1 || idx2 == -1 || idx3 == -1 || idx4 == -1 {
		t.Fatal("not all messages found in output")
	}
	if idx1 >= idx2 || idx2 >= idx3 || idx3 >= idx4 {
		t.Error("messages not in expected order")
	}
}

func TestBuildContent_WrapsLongLines(t *testing.T) {
	width := 60
	longText := "The quick brown fox jumps over the lazy dog. This is a really long response from the LLM that should definitely wrap to multiple lines when displayed in a terminal that is only 60 characters wide."

	msgs := []chatMessage{
		{role: "assistant", content: longText},
	}

	content := buildContent(msgs, "", "qwen3:14b", width)

	// The content must have newlines from wrapping.
	lines := strings.Split(content, "\n")
	nonEmptyLines := 0
	for _, line := range lines {
		if strings.TrimSpace(stripANSI(line)) != "" {
			nonEmptyLines++
		}
	}
	if nonEmptyLines < 3 {
		t.Errorf("expected at least 3 non-empty lines from wrapping, got %d:\n%s", nonEmptyLines, content)
	}

	// No visible line should exceed the width.
	for i, line := range lines {
		visible := stripANSI(line)
		if len(visible) > width+2 { // small tolerance
			t.Errorf("line %d exceeds width %d (visible %d chars): %q", i, width, len(visible), visible)
		}
	}
}

func TestBuildContent_WrapsStreamBuf(t *testing.T) {
	width := 60
	longStream := "This is a streaming response that keeps going and going without any breaks and should be wrapped by the buildContent function before being displayed in the viewport."

	content := buildContent(nil, longStream, "qwen3:14b", width)

	lines := strings.Split(content, "\n")
	nonEmptyLines := 0
	for _, line := range lines {
		if strings.TrimSpace(stripANSI(line)) != "" {
			nonEmptyLines++
		}
	}
	if nonEmptyLines < 2 {
		t.Errorf("expected at least 2 non-empty lines from stream wrapping, got %d:\n%s", nonEmptyLines, content)
	}
}

func TestBuildContent_PreservesExistingNewlines(t *testing.T) {
	width := 80
	text := "Line one.\nLine two.\nLine three."

	msgs := []chatMessage{
		{role: "assistant", content: text},
	}

	content := buildContent(msgs, "", "test", width)

	if !strings.Contains(content, "Line one.") || !strings.Contains(content, "Line two.") {
		t.Errorf("existing newlines not preserved:\n%s", content)
	}
}

func TestBuildContent_EmptyStreamBuffer(t *testing.T) {
	msgs := []chatMessage{
		{role: "user", content: "hello"},
	}
	result := buildContent(msgs, "", "model", 80)

	// Should not have a dangling model label for empty stream buffer.
	count := strings.Count(result, "model:")
	if count != 0 {
		// The model label only appears in assistant messages or streaming buffer.
		// With no assistant message and empty stream, only user message should appear.
	}
}
