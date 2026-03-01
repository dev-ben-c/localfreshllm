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
)

// RenderMascot renders mascot art with lipgloss styles (green leaf, yellow body).
func RenderMascot(art string) string {
	lines := splitLines(art)
	var result string
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		if i == 0 {
			result += mascotLeafStyle.Render(line)
		} else {
			result += mascotBodyStyle.Render(line)
		}
	}
	return result
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
