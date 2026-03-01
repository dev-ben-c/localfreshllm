package tui

import (
	"fmt"
	"regexp"
	"strconv"
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

// Word-to-number mapping for natural language parsing.
var wordNums = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
	"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
	"eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15,
	"sixteen": 16, "seventeen": 17, "eighteen": 18, "nineteen": 19, "twenty": 20,
	"thirty": 30, "forty": 40, "forty-five": 45, "forty five": 45,
	"fifty": 50, "sixty": 60, "ninety": 90,
	"a": 1, "an": 1,
	"half": -1, // sentinel for "half an hour" etc.
}

var reTimerNL = regexp.MustCompile(`(?i)(?:set\s+(?:a\s+)?timer\s+(?:for\s+)?|timer\s+(?:for\s+)?)(.+)`)

// parseNaturalTimer attempts to parse natural language timer requests.
// Returns a Timer pointer if matched, nil otherwise.
// Examples: "set a timer for 5 minutes", "timer for one hour",
// "set a timer for 1 minute called eggs", "set a 30 second timer"
func parseNaturalTimer(input string) *Timer {
	// Try "set a X minute timer" pattern first (more specific).
	if t := parseInlineTimer(input); t != nil {
		return t
	}

	// Try "set a timer for X minutes" / "timer for X" pattern.
	m := reTimerNL.FindStringSubmatch(input)
	if m == nil {
		return nil
	}

	rest := strings.TrimSpace(m[1])
	return parseTimerDuration(rest)
}

// parseInlineTimer handles "set a 5 minute timer" pattern.
var reInlineTimer = regexp.MustCompile(`(?i)set\s+(?:a\s+)?(.+?)\s+timer(?:\s+(?:called|named)\s+(.+))?$`)

func parseInlineTimer(input string) *Timer {
	m := reInlineTimer.FindStringSubmatch(input)
	if m == nil {
		return nil
	}
	t := parseTimerDuration(m[1])
	if t != nil && m[2] != "" {
		t.Name = strings.TrimSpace(m[2])
	}
	return t
}

// parseTimerDuration parses a duration phrase like "5 minutes", "one hour",
// "half an hour", "90 seconds", "1 minute called eggs".
func parseTimerDuration(phrase string) *Timer {
	phrase = strings.TrimRight(strings.TrimSpace(phrase), ".")
	lower := strings.ToLower(phrase)

	// Split off "called X" or "named X" for timer name.
	name := "timer"
	for _, sep := range []string{" called ", " named "} {
		if idx := strings.Index(lower, sep); idx >= 0 {
			name = strings.TrimSpace(phrase[idx+len(sep):])
			lower = lower[:idx]
			phrase = phrase[:idx]
		}
	}

	// Special case: "half an hour", "half hour"
	if strings.Contains(lower, "half") && strings.Contains(lower, "hour") {
		return &Timer{Name: name, Duration: 30 * time.Minute}
	}

	// Extract number + unit pairs.
	var total time.Duration
	parts := strings.Fields(lower)
	i := 0
	for i < len(parts) {
		// Get the number.
		num, advance := extractNumber(parts, i)
		if num < 0 || advance == 0 {
			return nil
		}
		i += advance

		// Optional "and" between pairs.
		if i < len(parts) && parts[i] == "and" {
			i++
		}

		// Get the unit.
		if i >= len(parts) {
			// Bare number — assume minutes.
			total += time.Duration(num) * time.Minute
			break
		}

		unit := parts[i]
		switch {
		case strings.HasPrefix(unit, "second"):
			total += time.Duration(num) * time.Second
		case strings.HasPrefix(unit, "minute"):
			total += time.Duration(num) * time.Minute
		case strings.HasPrefix(unit, "hour"):
			total += time.Duration(num) * time.Hour
		default:
			// Unknown unit — maybe it's a name or garbage.
			return nil
		}
		i++
	}

	if total <= 0 || total > maxDuration {
		return nil
	}
	return &Timer{Name: name, Duration: total}
}

// extractNumber tries to read a number from parts starting at idx.
// Returns the number and how many tokens were consumed.
func extractNumber(parts []string, idx int) (int, int) {
	if idx >= len(parts) {
		return -1, 0
	}
	word := parts[idx]

	// Try numeric.
	if n, err := strconv.Atoi(word); err == nil && n > 0 {
		return n, 1
	}

	// Try word number (handle "forty five" as two tokens).
	if n, ok := wordNums[word]; ok {
		if n == -1 {
			return -1, 0 // "half" — handled elsewhere
		}
		// Check for compound like "twenty five".
		if n >= 20 && idx+1 < len(parts) {
			if n2, ok := wordNums[parts[idx+1]]; ok && n2 > 0 && n2 < 10 {
				return n + n2, 2
			}
		}
		return n, 1
	}

	return -1, 0
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
