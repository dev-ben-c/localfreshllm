package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadHistory_NonexistentFile(t *testing.T) {
	// Point to a nonexistent path.
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	history := loadHistory()
	if history != nil {
		t.Errorf("expected nil for nonexistent file, got %v", history)
	}
}

func TestSaveAndLoadHistory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	input := []string{"hello", "world", "how are you"}
	saveHistory(input)

	loaded := loadHistory()
	if len(loaded) != len(input) {
		t.Fatalf("expected %d entries, got %d", len(input), len(loaded))
	}
	for i, s := range input {
		if loaded[i] != s {
			t.Errorf("entry %d: expected %q, got %q", i, s, loaded[i])
		}
	}
}

func TestSaveHistory_Truncation(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	// Create 250 entries — should truncate to 200.
	var entries []string
	for i := 0; i < 250; i++ {
		entries = append(entries, "entry")
	}
	saveHistory(entries)

	loaded := loadHistory()
	if len(loaded) != 200 {
		t.Errorf("expected 200 entries after truncation, got %d", len(loaded))
	}
}

func TestLoadHistory_SkipsEmptyLines(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	// Write a file with empty lines.
	dir := filepath.Join(tmpDir, "localfreshllm")
	os.MkdirAll(dir, 0700)
	data := "hello\n\nworld\n\n\n"
	os.WriteFile(filepath.Join(dir, ".tui_history"), []byte(data), 0600)

	loaded := loadHistory()
	if len(loaded) != 2 {
		t.Errorf("expected 2 non-empty entries, got %d", len(loaded))
	}
	if loaded[0] != "hello" || loaded[1] != "world" {
		t.Errorf("unexpected entries: %v", loaded)
	}
}

func TestHistoryPath(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/test-xdg")
	path := historyPath()
	if !strings.Contains(path, "/tmp/test-xdg/localfreshllm/.tui_history") {
		t.Errorf("unexpected path: %s", path)
	}
}
