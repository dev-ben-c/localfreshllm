package tui

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rabidclock/localfreshllm/audio"
	"github.com/rabidclock/localfreshllm/audio/capture"
	"github.com/rabidclock/localfreshllm/audio/playback"
	"github.com/rabidclock/localfreshllm/client"
)

// Tea messages for audio events.
type audioTranscribeDoneMsg struct {
	text string
	err  error
}

type audioPlayDoneMsg struct {
	err error
}

// transcribeAudio sends PCM data to Whisper for STT.
// Supports local Whisper server (via WhisperURL) or remote server (via client mode).
func transcribeAudio(cfg Config, pcm []byte) tea.Cmd {
	return func() tea.Msg {
		if cfg.WhisperURL != "" {
			whisper := audio.NewWhisperClient(cfg.WhisperURL)
			text, err := whisper.Transcribe(context.Background(), pcm)
			return audioTranscribeDoneMsg{text: text, err: err}
		}
		if remote, ok := cfg.Backend.(*client.RemoteBackend); ok {
			text, err := remote.Transcribe(context.Background(), pcm)
			return audioTranscribeDoneMsg{text: text, err: err}
		}
		return audioTranscribeDoneMsg{err: fmt.Errorf("no whisper server configured")}
	}
}

// playTTS synthesizes speech and plays it. Uses local piper if configured,
// otherwise falls back to the remote server's /v1/audio/speak endpoint.
func playTTS(cfg Config, p *playback.Player, text string) tea.Cmd {
	return func() tea.Msg {
		clean := sanitizeForTTS(text)
		if clean == "" {
			return audioPlayDoneMsg{}
		}

		var wavData []byte
		var err error

		if cfg.PiperModel != "" {
			piper := audio.NewPiperTTS(cfg.PiperModel, cfg.PiperSpeaker)
			wavData, err = piper.Speak(context.Background(), clean)
		} else if remote, ok := cfg.Backend.(*client.RemoteBackend); ok {
			wavData, err = remote.Speak(context.Background(), clean)
		} else {
			return audioPlayDoneMsg{}
		}

		if err != nil {
			return audioPlayDoneMsg{err: err}
		}
		err = p.Play(context.Background(), wavData)
		return audioPlayDoneMsg{err: err}
	}
}

var (
	// Markdown code blocks (fenced and inline).
	reCodeBlock = regexp.MustCompile("(?s)```.*?```")
	reInlineCode = regexp.MustCompile("`[^`]+`")
	// Markdown headers.
	reHeader = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	// Markdown bold/italic markers.
	reBoldItalic = regexp.MustCompile(`\*{1,3}`)
	// URLs.
	reURL = regexp.MustCompile(`https?://\S+`)
	// Bullet list markers.
	reBullet = regexp.MustCompile(`(?m)^\s*[-*+]\s+`)
	// Numbered list markers.
	reNumbered = regexp.MustCompile(`(?m)^\s*\d+\.\s+`)
	// Newlines (one or more) — converted to sentence breaks.
	reNewlines = regexp.MustCompile(`\n+`)
	// Multiple whitespace.
	reWhitespace = regexp.MustCompile(`\s+`)
	// Redundant periods from newline conversion (e.g. ".\n" → ".. " → ".").
	reMultiPeriod = regexp.MustCompile(`[.\s]*\.\s*\.`)
)

// sanitizeForTTS cleans LLM output for natural-sounding speech.
func sanitizeForTTS(text string) string {
	// Remove code blocks entirely — they sound terrible spoken.
	text = reCodeBlock.ReplaceAllString(text, " ")
	text = reInlineCode.ReplaceAllString(text, " ")

	// Remove URLs.
	text = reURL.ReplaceAllString(text, " ")

	// Remove markdown formatting.
	text = reHeader.ReplaceAllString(text, "")
	text = reBoldItalic.ReplaceAllString(text, "")
	text = reBullet.ReplaceAllString(text, "")
	text = reNumbered.ReplaceAllString(text, "")

	// Expand abbreviations and symbols for natural speech.
	text = expandForSpeech(text)

	// Remove characters that get read literally.
	text = strings.NewReplacer(
		"(", "", ")", "",
		"[", "", "]", "",
		"{", "", "}", "",
		"~", "", "_", " ",
		"|", "", ">", "",
		"#", "", "```", "",
	).Replace(text)

	// Convert newlines to sentence breaks for natural pauses.
	text = reNewlines.ReplaceAllString(text, ". ")

	// Strip emojis and other non-speech unicode.
	var b strings.Builder
	for _, r := range text {
		if isSpokenRune(r) {
			b.WriteRune(r)
		}
	}
	text = b.String()

	// Collapse whitespace and redundant punctuation.
	text = reWhitespace.ReplaceAllString(text, " ")
	text = reMultiPeriod.ReplaceAllString(text, ".")
	return strings.TrimSpace(text)
}

// US state abbreviations → full names.
var stateAbbrevs = map[string]string{
	"AL": "Alabama", "AK": "Alaska", "AZ": "Arizona", "AR": "Arkansas",
	"CA": "California", "CO": "Colorado", "CT": "Connecticut", "DE": "Delaware",
	"FL": "Florida", "GA": "Georgia", "HI": "Hawaii", "ID": "Idaho",
	"IL": "Illinois", "IN": "Indiana", "IA": "Iowa", "KS": "Kansas",
	"KY": "Kentucky", "LA": "Louisiana", "ME": "Maine", "MD": "Maryland",
	"MA": "Massachusetts", "MI": "Michigan", "MN": "Minnesota", "MS": "Mississippi",
	"MO": "Missouri", "MT": "Montana", "NE": "Nebraska", "NV": "Nevada",
	"NH": "New Hampshire", "NJ": "New Jersey", "NM": "New Mexico", "NY": "New York",
	"NC": "North Carolina", "ND": "North Dakota", "OH": "Ohio", "OK": "Oklahoma",
	"OR": "Oregon", "PA": "Pennsylvania", "RI": "Rhode Island", "SC": "South Carolina",
	"SD": "South Dakota", "TN": "Tennessee", "TX": "Texas", "UT": "Utah",
	"VT": "Vermont", "VA": "Virginia", "WA": "Washington", "WV": "West Virginia",
	"WI": "Wisconsin", "WY": "Wyoming", "DC": "District of Columbia",
}

// reStateAbbrev matches ", XX" where XX is a two-letter uppercase code (city, state pattern).
var reStateAbbrev = regexp.MustCompile(`(,\s*)([A-Z]{2})\b`)

// reDegrees matches temperature patterns like "18°C", "72°F", "18 °C".
var reDegrees = regexp.MustCompile(`(\d+)\s*°\s*([CFcf])`)

// rePercent matches "50%" patterns.
var rePercent = regexp.MustCompile(`(\d+)%`)

// Month abbreviations → full names.
var monthAbbrevs = map[string]string{
	"Jan": "January", "Feb": "February", "Mar": "March", "Apr": "April",
	"Jun": "June", "Jul": "July", "Aug": "August", "Sep": "September",
	"Oct": "October", "Nov": "November", "Dec": "December",
}

// reMonthDay matches "Mar 1", "Dec 25", "Jan 03" etc.
var reMonthDay = regexp.MustCompile(`\b(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\.?\s+(\d{1,2})\b`)

// expandForSpeech replaces abbreviations and symbols with spoken forms.
func expandForSpeech(text string) string {
	// Expand "City, FL" → "City, Florida".
	text = reStateAbbrev.ReplaceAllStringFunc(text, func(match string) string {
		m := reStateAbbrev.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		if full, ok := stateAbbrevs[m[2]]; ok {
			return m[1] + full
		}
		return match
	})

	// Expand "18°C" → "18 degrees Celsius", "72°F" → "72 degrees Fahrenheit".
	text = reDegrees.ReplaceAllStringFunc(text, func(match string) string {
		m := reDegrees.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		unit := "Celsius"
		if strings.ToUpper(m[2]) == "F" {
			unit = "Fahrenheit"
		}
		return m[1] + " degrees " + unit
	})

	// Expand "50%" → "50 percent".
	text = rePercent.ReplaceAllString(text, "${1} percent")

	// Expand "Mar 1" → "March 1st", "Dec 25" → "December 25th".
	text = reMonthDay.ReplaceAllStringFunc(text, func(match string) string {
		m := reMonthDay.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		month := m[1]
		if full, ok := monthAbbrevs[month]; ok {
			month = full
		}
		return month + " " + ordinal(m[2])
	})

	// Common abbreviations.
	text = strings.NewReplacer(
		"km/h", "kilometers per hour",
		"mph", "miles per hour",
		"m/s", "meters per second",
		"e.g.", "for example",
		"i.e.", "that is",
		"etc.", "etcetera",
		"vs.", "versus",
		"approx.", "approximately",
	).Replace(text)

	return text
}

// ordinalWords maps day numbers to their spoken form.
var ordinalWords = map[int]string{
	1: "first", 2: "second", 3: "third", 4: "fourth", 5: "fifth",
	6: "sixth", 7: "seventh", 8: "eighth", 9: "ninth", 10: "tenth",
	11: "eleventh", 12: "twelfth", 13: "thirteenth", 14: "fourteenth", 15: "fifteenth",
	16: "sixteenth", 17: "seventeenth", 18: "eighteenth", 19: "nineteenth", 20: "twentieth",
	21: "twenty first", 22: "twenty second", 23: "twenty third", 24: "twenty fourth",
	25: "twenty fifth", 26: "twenty sixth", 27: "twenty seventh", 28: "twenty eighth",
	29: "twenty ninth", 30: "thirtieth", 31: "thirty first",
}

// ordinal converts a day number string to its spoken word form: "1" → "first", "2" → "second", etc.
func ordinal(day string) string {
	n, err := strconv.Atoi(strings.TrimLeft(day, "0"))
	if err != nil || n < 1 || n > 31 {
		return day
	}
	if word, ok := ordinalWords[n]; ok {
		return word
	}
	return day
}

// isSpokenRune returns true for characters that make sense in spoken text.
func isSpokenRune(r rune) bool {
	if r <= 127 {
		return true // ASCII
	}
	// Allow common Latin/extended characters and CJK.
	if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsPunct(r) {
		// Reject emoji-range punctuation/symbols.
		if r >= 0x2600 {
			return false
		}
		return true
	}
	if unicode.IsSpace(r) {
		return true
	}
	return false
}

// AudioAvailable returns whether mic capture and playback tools are installed.
func AudioAvailable() (mic bool, speaker bool) {
	return capture.Available(), playback.Available()
}
