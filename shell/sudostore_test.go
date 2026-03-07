package shell

import (
	"testing"
	"time"
)

func TestSudoStoreSetGet(t *testing.T) {
	s := NewSudoStore()
	s.Set("dev1", "secret")
	if got := s.Get("dev1"); got != "secret" {
		t.Errorf("expected 'secret', got %q", got)
	}
}

func TestSudoStoreExpiry(t *testing.T) {
	s := NewSudoStore()
	s.mu.Lock()
	s.cache["dev1"] = &sudoEntry{
		password:  "old",
		expiresAt: time.Now().Add(-1 * time.Second),
	}
	s.mu.Unlock()

	if got := s.Get("dev1"); got != "" {
		t.Errorf("expected empty (expired), got %q", got)
	}
}

func TestSudoStoreClear(t *testing.T) {
	s := NewSudoStore()
	s.Set("dev1", "secret")
	s.Clear("dev1")
	if got := s.Get("dev1"); got != "" {
		t.Errorf("expected empty after clear, got %q", got)
	}
}

func TestSudoStoreUnknownDevice(t *testing.T) {
	s := NewSudoStore()
	if got := s.Get("nonexistent"); got != "" {
		t.Errorf("expected empty for unknown device, got %q", got)
	}
}

func TestContainsSudo(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"sudo apt update", true},
		{"echo foo && sudo reboot", true},
		{"ls -la", false},
		{"sudoers", false},
		{"SUDO_ASKPASS=x sudo -A cmd", true},
	}
	for _, tt := range tests {
		if got := containsSudo(tt.cmd); got != tt.want {
			t.Errorf("containsSudo(%q) = %v, want %v", tt.cmd, got, tt.want)
		}
	}
}
