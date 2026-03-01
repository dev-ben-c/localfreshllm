package backend

import (
	"testing"
)

func TestForModel_Anthropic(t *testing.T) {
	b := ForModel("claude-sonnet-4-6")
	if _, ok := b.(*Anthropic); !ok {
		t.Errorf("expected *Anthropic for claude model, got %T", b)
	}
}

func TestForModel_Ollama(t *testing.T) {
	b := ForModel("qwen3:32b")
	if _, ok := b.(*Ollama); !ok {
		t.Errorf("expected *Ollama for non-claude model, got %T", b)
	}
}

func TestForModel_OllamaDefault(t *testing.T) {
	b := ForModel("llama3:8b")
	if _, ok := b.(*Ollama); !ok {
		t.Errorf("expected *Ollama for llama model, got %T", b)
	}
}

func TestAnthropic_Validate_NoKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	a := NewAnthropic()
	if err := a.Validate(); err == nil {
		t.Errorf("expected error when ANTHROPIC_API_KEY is not set")
	}
}

func TestAnthropic_Validate_WithKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key")
	a := NewAnthropic()
	if err := a.Validate(); err != nil {
		t.Errorf("unexpected error with key set: %v", err)
	}
}
