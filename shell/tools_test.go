package shell

import (
	"context"
	"testing"
)

func TestExecBasic(t *testing.T) {
	e := NewExecutor()
	result := e.Execute(context.Background(), ToolCall{
		ID:   "1",
		Name: "shell_exec",
		Args: map[string]any{"command": "echo hello"},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Name != "shell_exec" {
		t.Errorf("expected name shell_exec, got %s", result.Name)
	}
}

func TestExecExitCode(t *testing.T) {
	e := NewExecutor()
	result := e.Execute(context.Background(), ToolCall{
		ID:   "1",
		Name: "shell_exec",
		Args: map[string]any{"command": "exit 42"},
	})

	if !result.IsError {
		t.Error("expected error for non-zero exit code")
	}
}

func TestExecTimeout(t *testing.T) {
	e := NewExecutor()
	result := e.Execute(context.Background(), ToolCall{
		ID:   "1",
		Name: "shell_exec",
		Args: map[string]any{"command": "sleep 10", "timeout": float64(1)},
	})

	if !result.IsError {
		t.Error("expected timeout error")
	}
}

func TestExecMissingCommand(t *testing.T) {
	e := NewExecutor()
	result := e.Execute(context.Background(), ToolCall{
		ID:   "1",
		Name: "shell_exec",
		Args: map[string]any{},
	})

	if !result.IsError {
		t.Error("expected error for missing command")
	}
}

func TestExecStderr(t *testing.T) {
	e := NewExecutor()
	result := e.Execute(context.Background(), ToolCall{
		ID:   "1",
		Name: "shell_exec",
		Args: map[string]any{"command": "echo err >&2 && echo out"},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
}
