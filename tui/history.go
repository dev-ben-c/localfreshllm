package tui

import (
	"os"
	"path/filepath"
	"strings"
)

func historyPath() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		dir = os.ExpandEnv("$HOME/.local/share")
	}
	return filepath.Join(dir, "localfreshllm", ".tui_history")
}

func loadHistory() []string {
	data, err := os.ReadFile(historyPath())
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var result []string
	for _, line := range lines {
		if line != "" {
			result = append(result, line)
		}
	}
	// Keep only last 200 entries.
	if len(result) > 200 {
		result = result[len(result)-200:]
	}
	return result
}

func saveHistory(history []string) {
	path := historyPath()
	os.MkdirAll(filepath.Dir(path), 0700)

	// Keep only last 200.
	if len(history) > 200 {
		history = history[len(history)-200:]
	}

	data := strings.Join(history, "\n") + "\n"
	os.WriteFile(path, []byte(data), 0600)
}
