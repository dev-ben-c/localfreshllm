package render

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Lemon mascot states using Braille dot art.
const (
	LemonIdle = "" +
		"        ⣠⠤⡀\n" +
		"      ⢀⣶⣿⣿⣶⡀\n" +
		"     ⣾⣿⠶⠀⠶⣿⣷\n" +
		"     ⣿⣿⡠⣀⢄⣿⣿\n" +
		"     ⠻⣿⣦⣤⣴⣿⠟\n" +
		"      ⠈⠛⠿⠿⠛⠁\n"

	LemonThinking1 = "" +
		"        ⣠⠤⡀  ⠄⠂⠁\n" +
		"      ⢀⣶⣿⣿⣶⡀\n" +
		"     ⣾⣿⠒⠀⠒⣿⣷\n" +
		"     ⣿⣿⠀⠤⠀⣿⣿\n" +
		"     ⠻⣿⣦⣤⣴⣿⠟\n" +
		"      ⠈⠛⠿⠿⠛⠁\n"

	LemonThinking2 = "" +
		"        ⣠⠤⡀  ⠂⠁⠄\n" +
		"      ⢀⣶⣿⣿⣶⡀\n" +
		"     ⣾⣿⠒⠀⠒⣿⣷\n" +
		"     ⣿⣿⠀⠤⠀⣿⣿\n" +
		"     ⠻⣿⣦⣤⣴⣿⠟\n" +
		"      ⠈⠛⠿⠿⠛⠁\n"

	LemonThinking3 = "" +
		"        ⣠⠤⡀  ⠁⠄⠂\n" +
		"      ⢀⣶⣿⣿⣶⡀\n" +
		"     ⣾⣿⠒⠀⠒⣿⣷\n" +
		"     ⣿⣿⠀⠤⠀⣿⣿\n" +
		"     ⠻⣿⣦⣤⣴⣿⠟\n" +
		"      ⠈⠛⠿⠿⠛⠁\n"

	// LemonThinking is kept for backward compatibility (alias for frame 1).
	LemonThinking = LemonThinking1

	LemonSpeaking = "" +
		"        ⣠⠤⡀\n" +
		"      ⢀⣶⣿⣿⣶⡀\n" +
		"     ⣾⣿⠶⠀⠶⣿⣷\n" +
		"     ⣿⣿⠰⠶⠆⣿⣿\n" +
		"     ⠻⣿⣦⣤⣴⣿⠟\n" +
		"      ⠈⠛⠿⠿⠛⠁\n"
)

// LemonThinkingFrames returns all thinking animation frames.
func LemonThinkingFrames() []string {
	return []string{LemonThinking1, LemonThinking2, LemonThinking3}
}

var (
	mascotLeafStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	mascotBodyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	mascotFaceStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
)

// RenderMascot renders mascot art with lipgloss styles (green leaf, yellow body, white face).
func RenderMascot(art string) string {
	lines := splitLines(art)
	var result string
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		if i == 0 {
			result += mascotLeafStyle.Render(line)
		} else if i == 2 || i == 3 {
			result += renderMascotFaceLine(line)
		} else {
			result += mascotBodyStyle.Render(line)
		}
	}
	return result
}

// renderMascotFaceLine renders a body line with the center 3 face characters in white.
func renderMascotFaceLine(line string) string {
	runes := []rune(line)
	if len(runes) < 11 {
		return mascotBodyStyle.Render(line)
	}
	return mascotBodyStyle.Render(string(runes[:7])) + mascotFaceStyle.Render(string(runes[7:10])) + mascotBodyStyle.Render(string(runes[10:]))
}

// PrintLemonColored prints the lemon with the leaf in green and body in yellow.
func PrintLemonColored(art string) {
	fmt.Println(RenderMascot(art))
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, ch := range s {
		if ch == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
