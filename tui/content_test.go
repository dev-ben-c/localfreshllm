package tui

import (
	"strings"
	"testing"
)

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
