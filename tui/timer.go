package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	maxTimers   = 5
	maxDuration = 24 * time.Hour
)

// Timer represents a countdown timer.
type Timer struct {
	Name     string
	Duration time.Duration
	Deadline time.Time
}

// Remaining returns the time left on the timer.
func (t Timer) Remaining() time.Duration {
	r := time.Until(t.Deadline)
	if r < 0 {
		return 0
	}
	return r
}

// Expired returns true if the timer has finished.
func (t Timer) Expired() bool {
	return time.Now().After(t.Deadline)
}

// timerTickMsg is sent every second to update timer displays.
type timerTickMsg time.Time

// timerExpiredMsg signals that one or more timers have expired.
type timerExpiredMsg struct {
	names []string
}

// timerTick returns a command that ticks every second.
func timerTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return timerTickMsg(t)
	})
}

// parseDuration parses a human-friendly duration string like "5m", "1h30m", "90s".
// Also supports bare numbers as minutes (e.g. "5" = 5 minutes).
func parseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}
	// Try as bare number (minutes).
	var mins float64
	if _, scanErr := fmt.Sscanf(s, "%f", &mins); scanErr == nil && mins > 0 {
		return time.Duration(mins * float64(time.Minute)), nil
	}
	return 0, fmt.Errorf("invalid duration %q (try 5m, 1h30m, 90s, or a number for minutes)", s)
}

// formatRemaining formats a duration for compact display.
func formatRemaining(d time.Duration) string {
	if d <= 0 {
		return "0:00"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// renderTimerStatus returns a compact status line for the header.
func renderTimerStatus(timers []Timer) string {
	if len(timers) == 0 {
		return ""
	}
	var parts []string
	for _, t := range timers {
		parts = append(parts, fmt.Sprintf("%s %s", t.Name, formatRemaining(t.Remaining())))
	}
	return strings.Join(parts, " | ")
}

// checkExpired removes expired timers from the slice and returns their names.
func checkExpired(timers *[]Timer) []string {
	var expired []string
	active := (*timers)[:0]
	for _, t := range *timers {
		if t.Expired() {
			expired = append(expired, t.Name)
		} else {
			active = append(active, t)
		}
	}
	*timers = active
	return expired
}
