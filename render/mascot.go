package render

import "fmt"

// Lemon mascot states using Braille dot art.
const (
	LemonIdle = "" +
		"        ⣠⠤⡀\n" +
		"      ⢀⣶⣿⣿⣶⡀\n" +
		"     ⣾⣿⡇⠀⢸⣿⣷\n" +
		"     ⣿⣿⠀⠀⠀⣿⣿\n" +
		"     ⠻⣿⣦⣤⣴⣿⠟\n" +
		"      ⠈⠛⠿⠿⠛⠁\n"

	LemonThinking = "" +
		"        ⣠⠤⡀  ⠄⠂⠁\n" +
		"      ⢀⣶⣿⣿⣶⡀\n" +
		"     ⣾⣿⠃⠀⠘⣿⣷\n" +
		"     ⣿⣿⠀⠀⠀⣿⣿\n" +
		"     ⠻⣿⣦⠤⣴⣿⠟\n" +
		"      ⠈⠛⠿⠿⠛⠁\n"

	LemonSpeaking = "" +
		"        ⣠⠤⡀\n" +
		"      ⢀⣶⣿⣿⣶⡀\n" +
		"     ⣾⣿⡇⠀⢸⣿⣷\n" +
		"     ⣿⣿⠀⣀⠀⣿⣿\n" +
		"     ⠻⣿⣦⠛⣴⣿⠟\n" +
		"      ⠈⠛⠿⠿⠛⠁\n"
)

// PrintLemonColored prints the lemon with the leaf in green and body in yellow.
func PrintLemonColored(art string) {
	green := "\033[32m"
	yellow := "\033[33m"
	reset := "\033[0m"

	lines := splitLines(art)
	for i, line := range lines {
		if i == 0 {
			fmt.Println(green + line + reset)
		} else {
			fmt.Println(yellow + line + reset)
		}
	}
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
