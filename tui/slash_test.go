package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/dev-ben-c/localfreshllm/backend"
	"github.com/dev-ben-c/localfreshllm/session"
)

// mockBackend implements backend.Backend for testing.
type mockBackend struct{}

func (m *mockBackend) Chat(_ context.Context, _ string, _ []backend.Message, _ string, _ []any, _ backend.StreamCallback) (*backend.ChatResult, error) {
	return &backend.ChatResult{Text: "mock"}, nil
}
func (m *mockBackend) ListModels(_ context.Context) ([]string, error) {
	return []string{"model-a", "model-b"}, nil
}
func (m *mockBackend) Validate() error { return nil }

func newTestConfig() *Config {
	return &Config{
		Backend:     &mockBackend{},
		UserConfig:  &session.Config{},
		Session:     session.NewSession("test123", "test-model"),
		Model:       "test-model",
		EnableTools: true,
	}
}

func TestHandleSlash_Quit(t *testing.T) {
	cfg := newTestConfig()

	for _, cmd := range []string{"/quit", "/exit", "/q"} {
		result := handleSlash(cmd, cfg)
		if !result.quit {
			t.Errorf("%s: expected quit=true", cmd)
		}
	}
}

func TestHandleSlash_Clear(t *testing.T) {
	cfg := newTestConfig()
	cfg.Session.AddMessage("user", "hello")

	result := handleSlash("/clear", cfg)

	if result.quit {
		t.Error("clear should not quit")
	}
	if !strings.Contains(result.info, "cleared") {
		t.Errorf("expected 'cleared' in info, got %q", result.info)
	}
	if len(cfg.Session.Messages) != 0 {
		t.Errorf("expected 0 messages after clear, got %d", len(cfg.Session.Messages))
	}
}

func TestHandleSlash_ToolsToggle(t *testing.T) {
	cfg := newTestConfig()
	cfg.EnableTools = true

	result := handleSlash("/tools", cfg)
	if cfg.EnableTools {
		t.Error("expected tools disabled after toggle")
	}
	if !strings.Contains(result.info, "disabled") {
		t.Errorf("expected 'disabled' in info, got %q", result.info)
	}

	result = handleSlash("/tools", cfg)
	if !cfg.EnableTools {
		t.Error("expected tools enabled after second toggle")
	}
	if !strings.Contains(result.info, "enabled") {
		t.Errorf("expected 'enabled' in info, got %q", result.info)
	}
}

func TestHandleSlash_LocationNoArgs(t *testing.T) {
	cfg := newTestConfig()
	cfg.UserConfig.Location = ""

	result := handleSlash("/location", cfg)
	if !strings.Contains(result.info, "No location set") {
		t.Errorf("expected 'No location set', got %q", result.info)
	}
}

func TestHandleSlash_LocationShowCurrent(t *testing.T) {
	cfg := newTestConfig()
	cfg.UserConfig.Location = "Baltimore"

	result := handleSlash("/location", cfg)
	if !strings.Contains(result.info, "Baltimore") {
		t.Errorf("expected current location in info, got %q", result.info)
	}
}

func TestHandleSlash_Help(t *testing.T) {
	cfg := newTestConfig()
	result := handleSlash("/help", cfg)

	if result.quit {
		t.Error("help should not quit")
	}

	// Check key commands are listed.
	for _, expected := range []string{"/model", "/clear", "/tools", "/quit", "/location", "/voice", "/tts"} {
		if !strings.Contains(result.info, expected) {
			t.Errorf("help text missing %q", expected)
		}
	}
}

func TestHandleSlash_UnknownCommand(t *testing.T) {
	cfg := newTestConfig()
	result := handleSlash("/nonexistent", cfg)

	if result.quit {
		t.Error("unknown command should not quit")
	}
	if !strings.Contains(result.info, "Unknown command") {
		t.Errorf("expected 'Unknown command', got %q", result.info)
	}
}

func TestHandleSlash_EmptyInput(t *testing.T) {
	cfg := newTestConfig()
	result := handleSlash("", cfg)

	if result.quit {
		t.Error("empty input should not quit")
	}
}

func TestHandleSlash_History(t *testing.T) {
	cfg := newTestConfig()
	result := handleSlash("/history", cfg)

	if result.quit {
		t.Error("history should not quit")
	}
	if result.info == "" {
		t.Error("expected non-empty info from /history")
	}
}

func TestHandleSlash_ModelPickerNoArgs(t *testing.T) {
	cfg := newTestConfig()
	result := handleSlash("/model", cfg)

	if result.quit {
		t.Error("model picker should not quit")
	}
	// Should either show picker or show model list info.
	if result.info == "" && !result.modelPick {
		t.Error("expected either info or modelPick from /model")
	}
}

func TestHelpText(t *testing.T) {
	text := helpText()

	if text == "" {
		t.Error("helpText should not be empty")
	}

	lines := strings.Split(text, "\n")
	if len(lines) < 5 {
		t.Errorf("expected at least 5 lines in help, got %d", len(lines))
	}
}

func TestHandleClear_ClientMode(t *testing.T) {
	cfg := newTestConfig()
	cfg.IsClient = true
	cfg.Session = nil

	result := handleClear(cfg)
	if !strings.Contains(result.info, "cleared") {
		t.Errorf("expected 'cleared' in info, got %q", result.info)
	}
}
