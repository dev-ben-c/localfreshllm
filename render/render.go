package render

import (
	"fmt"
	"os"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	// Pre-set color profile and background to prevent lipgloss/termenv from
	// querying the terminal via OSC escape sequences, which blocks in some
	// PTY environments that don't respond to those queries.
	lipgloss.SetColorProfile(termenv.ANSI256)
	lipgloss.SetHasDarkBackground(true)
}

var (
	// UserStyle for the "You:" prompt label.
	UserStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			Bold(true)

	// AssistantStyle for the model name label.
	AssistantStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("5")).
			Bold(true)

	// SystemStyle for system/info messages.
	SystemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	// ErrorStyle for error messages.
	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")).
			Bold(true)

	// ModelStyle for highlighting model names.
	ModelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3"))

	// DimStyle for secondary information.
	DimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	// TimerStyle for active timer countdowns.
	TimerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3")).
			Bold(true)

	// ToolStyle for tool names in the status bar.
	ToolStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))
)

// RenderMarkdown renders markdown text using glamour with dark style.
// Width controls word wrapping; 0 defaults to 80.
func RenderMarkdown(text string, width int) string {
	if width <= 0 {
		width = 80
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return text
	}

	out, err := r.Render(text)
	if err != nil {
		return text
	}

	return out
}

// Errorf prints a styled error message to stderr.
func Errorf(format string, args ...any) {
	fmt.Fprintln(os.Stderr, ErrorStyle.Render(fmt.Sprintf(format, args...)))
}

// Infof prints a styled info message to stderr.
func Infof(format string, args ...any) {
	fmt.Fprintln(os.Stderr, SystemStyle.Render(fmt.Sprintf(format, args...)))
}
