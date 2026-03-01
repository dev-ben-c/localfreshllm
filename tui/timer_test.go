package tui

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
		err   bool
	}{
		{"5m", 5 * time.Minute, false},
		{"1h30m", 90 * time.Minute, false},
		{"90s", 90 * time.Second, false},
		{"1h", time.Hour, false},
		{"2h30m15s", 2*time.Hour + 30*time.Minute + 15*time.Second, false},
		{"5", 5 * time.Minute, false},      // bare number = minutes
		{"0.5", 30 * time.Second, false},    // fractional minutes
		{"", 0, true},                        // empty
		{"abc", 0, true},                     // invalid
		{"-5m", -5 * time.Minute, false},     // Go parses negative, caller validates
	}

	for _, tt := range tests {
		d, err := parseDuration(tt.input)
		if tt.err && err == nil {
			t.Errorf("parseDuration(%q): expected error, got %v", tt.input, d)
		}
		if !tt.err && err != nil {
			t.Errorf("parseDuration(%q): unexpected error: %v", tt.input, err)
		}
		if !tt.err && d != tt.want {
			t.Errorf("parseDuration(%q) = %v, want %v", tt.input, d, tt.want)
		}
	}
}

func TestFormatRemaining(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00"},
		{-time.Second, "0:00"},
		{30 * time.Second, "0:30"},
		{5*time.Minute + 30*time.Second, "5:30"},
		{59*time.Minute + 59*time.Second, "59:59"},
		{time.Hour, "1:00:00"},
		{2*time.Hour + 30*time.Minute + 15*time.Second, "2:30:15"},
		{23*time.Hour + 59*time.Minute + 59*time.Second, "23:59:59"},
	}

	for _, tt := range tests {
		got := formatRemaining(tt.d)
		if got != tt.want {
			t.Errorf("formatRemaining(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestTimerExpired(t *testing.T) {
	past := Timer{Name: "test", Deadline: time.Now().Add(-time.Second)}
	if !past.Expired() {
		t.Error("expected timer in the past to be expired")
	}

	future := Timer{Name: "test", Deadline: time.Now().Add(time.Hour)}
	if future.Expired() {
		t.Error("expected timer in the future to not be expired")
	}
}

func TestTimerRemaining(t *testing.T) {
	past := Timer{Deadline: time.Now().Add(-time.Second)}
	if past.Remaining() != 0 {
		t.Errorf("expired timer remaining = %v, want 0", past.Remaining())
	}

	future := Timer{Deadline: time.Now().Add(5 * time.Minute)}
	r := future.Remaining()
	if r < 4*time.Minute || r > 5*time.Minute+time.Second {
		t.Errorf("remaining = %v, expected ~5m", r)
	}
}

func TestCheckExpired(t *testing.T) {
	timers := []Timer{
		{Name: "done1", Deadline: time.Now().Add(-time.Second)},
		{Name: "active", Deadline: time.Now().Add(time.Hour)},
		{Name: "done2", Deadline: time.Now().Add(-time.Minute)},
	}

	expired := checkExpired(&timers)
	if len(expired) != 2 {
		t.Fatalf("expected 2 expired, got %d: %v", len(expired), expired)
	}
	if len(timers) != 1 {
		t.Fatalf("expected 1 remaining timer, got %d", len(timers))
	}
	if timers[0].Name != "active" {
		t.Errorf("remaining timer = %q, want %q", timers[0].Name, "active")
	}
}

func TestRenderTimerStatus(t *testing.T) {
	if s := renderTimerStatus(nil); s != "" {
		t.Errorf("empty timers should render empty, got %q", s)
	}

	timers := []Timer{
		{Name: "eggs", Deadline: time.Now().Add(5*time.Minute + 30*time.Second)},
		{Name: "laundry", Deadline: time.Now().Add(time.Hour)},
	}
	s := renderTimerStatus(timers)
	if s == "" {
		t.Error("expected non-empty status for active timers")
	}
	// Should contain both names.
	if !containsAll(s, "eggs", "laundry") {
		t.Errorf("status %q missing timer names", s)
	}
}

func TestMaxTimers(t *testing.T) {
	if maxTimers != 5 {
		t.Errorf("maxTimers = %d, want 5", maxTimers)
	}
}

func TestMaxDuration(t *testing.T) {
	if maxDuration != 24*time.Hour {
		t.Errorf("maxDuration = %v, want 24h", maxDuration)
	}
}

func TestHandleTimerSlash(t *testing.T) {
	// /timer 5m
	r := handleTimer([]string{"5m"})
	if r.timerAdd == nil {
		t.Fatal("expected timerAdd for /timer 5m")
	}
	if r.timerAdd.Duration != 5*time.Minute {
		t.Errorf("duration = %v, want 5m", r.timerAdd.Duration)
	}
	if r.timerAdd.Name != "timer" {
		t.Errorf("name = %q, want 'timer'", r.timerAdd.Name)
	}

	// /timer 1h eggs
	r = handleTimer([]string{"1h", "eggs"})
	if r.timerAdd == nil {
		t.Fatal("expected timerAdd for /timer 1h eggs")
	}
	if r.timerAdd.Name != "eggs" {
		t.Errorf("name = %q, want 'eggs'", r.timerAdd.Name)
	}

	// /timer 25h → too long
	r = handleTimer([]string{"25h"})
	if r.timerAdd != nil {
		t.Error("expected nil timerAdd for duration > 24h")
	}
	if r.info == "" {
		t.Error("expected error info for duration > 24h")
	}

	// /timer cancel 1
	r = handleTimer([]string{"cancel", "1"})
	if r.timerCancel != 1 {
		t.Errorf("timerCancel = %d, want 1", r.timerCancel)
	}

	// /timer clear
	r = handleTimer([]string{"clear"})
	if !r.timerClear {
		t.Error("expected timerClear")
	}

	// /timer list
	r = handleTimer([]string{"list"})
	if r.info != "_timer_list_" {
		t.Errorf("expected _timer_list_ sentinel, got %q", r.info)
	}

	// /timer (no args)
	r = handleTimer(nil)
	if r.info != "_timer_list_" {
		t.Errorf("expected _timer_list_ sentinel for empty args, got %q", r.info)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
