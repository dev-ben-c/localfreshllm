package files

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePath(t *testing.T) {
	tmp := t.TempDir()
	e := NewExecutor([]string{tmp})

	// Allowed path.
	if _, err := e.validatePath(filepath.Join(tmp, "test.txt")); err != nil {
		t.Errorf("expected allowed, got: %v", err)
	}

	// Traversal attempt.
	if _, err := e.validatePath(filepath.Join(tmp, "..", "etc", "passwd")); err == nil {
		t.Error("expected traversal to be blocked")
	}

	// Outside allowed path entirely.
	if _, err := e.validatePath("/etc/passwd"); err == nil {
		t.Error("expected /etc/passwd to be blocked")
	}

	// Relative path.
	if _, err := e.validatePath("relative/path"); err == nil {
		t.Error("expected relative path to be rejected")
	}

	// Exact match of allowed base.
	if _, err := e.validatePath(tmp); err != nil {
		t.Errorf("expected base path itself to be allowed, got: %v", err)
	}
}

func TestValidatePathSymlink(t *testing.T) {
	tmp := t.TempDir()
	allowed := filepath.Join(tmp, "allowed")
	outside := filepath.Join(tmp, "outside")
	os.MkdirAll(allowed, 0755)
	os.MkdirAll(outside, 0755)
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0644)

	// Create symlink inside allowed pointing outside.
	link := filepath.Join(allowed, "escape")
	os.Symlink(outside, link)

	e := NewExecutor([]string{allowed})

	// Following the symlink should resolve to outside and be blocked.
	if _, err := e.validatePath(filepath.Join(link, "secret.txt")); err == nil {
		t.Error("expected symlink escape to be blocked")
	}
}

func TestExecReadBasic(t *testing.T) {
	tmp := t.TempDir()
	testFile := filepath.Join(tmp, "hello.txt")
	os.WriteFile(testFile, []byte("line1\nline2\nline3\n"), 0644)

	e := NewExecutor([]string{tmp})
	result := e.Execute(context.Background(), ToolCall{
		ID:   "1",
		Name: "file_read",
		Args: map[string]any{"path": testFile},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Name != "file_read" {
		t.Errorf("expected name file_read, got %s", result.Name)
	}
}

func TestExecListBasic(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("hello"), 0644)
	os.MkdirAll(filepath.Join(tmp, "subdir"), 0755)

	e := NewExecutor([]string{tmp})
	result := e.Execute(context.Background(), ToolCall{
		ID:   "1",
		Name: "file_list",
		Args: map[string]any{"path": tmp},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
}

func TestExecWriteAndRead(t *testing.T) {
	tmp := t.TempDir()
	testFile := filepath.Join(tmp, "new.txt")

	e := NewExecutor([]string{tmp})

	// Write.
	wr := e.Execute(context.Background(), ToolCall{
		ID:   "1",
		Name: "file_write",
		Args: map[string]any{"path": testFile, "content": "hello world"},
	})
	if wr.IsError {
		t.Fatalf("write error: %s", wr.Content)
	}

	// Read back.
	data, _ := os.ReadFile(testFile)
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestExecWriteOutsideAllowed(t *testing.T) {
	tmp := t.TempDir()
	e := NewExecutor([]string{tmp})

	result := e.Execute(context.Background(), ToolCall{
		ID:   "1",
		Name: "file_write",
		Args: map[string]any{"path": "/tmp/evil.txt", "content": "bad"},
	})
	if !result.IsError {
		t.Error("expected write outside allowed to fail")
	}
}
