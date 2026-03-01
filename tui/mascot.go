package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mascotState int

const (
	mascotIdle mascotState = iota
	mascotThinking
	mascotSpeaking
)

// MascotModel handles the animated lemon mascot.
type MascotModel struct {
	state mascotState
	frame int
}

type mascotTickMsg time.Time

var (
	leafStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	bodyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	faceStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
)

// renderBodyLine renders a mascot body line, with white face on the eyes/mouth rows.
func renderBodyLine(line string, hasFace bool) string {
	if !hasFace {
		return bodyStyle.Render(line)
	}
	runes := []rune(line)
	if len(runes) < 11 {
		return bodyStyle.Render(line)
	}
	return bodyStyle.Render(string(runes[:7])) + faceStyle.Render(string(runes[7:10])) + bodyStyle.Render(string(runes[10:]))
}

// Thinking frames with rotating dot patterns.
var thinkingDots = []string{
	"  ⠄⠂⠁",
	"  ⠂⠁⠄",
	"  ⠁⠄⠂",
}

// Base mascot art (without the first line).
var mascotBody = []string{
	"      ⢀⣶⣿⣿⣶⡀",
	"     ⣾⣿⠶⠀⠶⣿⣷",
	"     ⣿⣿⡠⣀⢄⣿⣿",
	"     ⠻⣿⣦⣤⣴⣿⠟",
	"      ⠈⠛⠿⠿⠛⠁",
}

var thinkingBody = []string{
	"      ⢀⣶⣿⣿⣶⡀",
	"     ⣾⣿⠒⠀⠒⣿⣷",
	"     ⣿⣿⠀⠤⠀⣿⣿",
	"     ⠻⣿⣦⣤⣴⣿⠟",
	"      ⠈⠛⠿⠿⠛⠁",
}

var speakingBody = []string{
	"      ⢀⣶⣿⣿⣶⡀",
	"     ⣾⣿⠶⠀⠶⣿⣷",
	"     ⣿⣿⠰⠶⠆⣿⣿",
	"     ⠻⣿⣦⣤⣴⣿⠟",
	"      ⠈⠛⠿⠿⠛⠁",
}

func NewMascotModel() MascotModel {
	return MascotModel{state: mascotIdle}
}

func mascotTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return mascotTickMsg(t)
	})
}

func (m MascotModel) Update(msg tea.Msg) (MascotModel, tea.Cmd) {
	if _, ok := msg.(mascotTickMsg); ok {
		if m.state == mascotThinking {
			m.frame = (m.frame + 1) % len(thinkingDots)
		}
		return m, mascotTick()
	}
	return m, nil
}

func (m MascotModel) View() string {
	var lines []string

	switch m.state {
	case mascotThinking:
		dots := thinkingDots[m.frame]
		lines = append(lines, leafStyle.Render("        ⣠⠤⡀")+bodyStyle.Render(dots))
		for i, line := range thinkingBody {
			lines = append(lines, renderBodyLine(line, i == 1 || i == 2))
		}
	case mascotSpeaking:
		lines = append(lines, leafStyle.Render("        ⣠⠤⡀"))
		for i, line := range speakingBody {
			lines = append(lines, renderBodyLine(line, i == 1 || i == 2))
		}
	default:
		lines = append(lines, leafStyle.Render("        ⣠⠤⡀"))
		for i, line := range mascotBody {
			lines = append(lines, renderBodyLine(line, i == 1 || i == 2))
		}
	}

	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}
