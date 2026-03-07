package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/dev-ben-c/localfreshllm/render"
)

var (
	userMsgStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	assistantMsgStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	systemMsgStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errorMsgStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
)

// buildContent renders the chat messages plus any in-progress stream into a single string.
func buildContent(messages []chatMessage, streamBuf string, model string, width int) string {
	if width <= 0 {
		width = 80
	}

	wrapStyle := lipgloss.NewStyle().Width(width)

	var sb strings.Builder

	for _, msg := range messages {
		var line string
		switch msg.role {
		case "user":
			line = render.UserStyle.Render("You: ") + userMsgStyle.Render(msg.content)
		case "assistant":
			line = render.AssistantStyle.Render(model+": ") + assistantMsgStyle.Render(msg.content)
		case "system":
			line = systemMsgStyle.Render(msg.content)
		case "error":
			line = errorMsgStyle.Render("Error: " + msg.content)
		}
		sb.WriteString(wrapStyle.Render(line))
		sb.WriteString("\n\n")
	}

	// Show in-progress streaming text.
	if streamBuf != "" {
		line := render.AssistantStyle.Render(model+": ") + assistantMsgStyle.Render(streamBuf)
		sb.WriteString(wrapStyle.Render(line))
		sb.WriteString("\n")
	}

	return sb.String()
}
