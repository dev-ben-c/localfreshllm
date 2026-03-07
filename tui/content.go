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
	var sb strings.Builder

	for _, msg := range messages {
		switch msg.role {
		case "user":
			sb.WriteString(render.UserStyle.Render("You: "))
			sb.WriteString(userMsgStyle.Render(msg.content))
		case "assistant":
			sb.WriteString(render.AssistantStyle.Render(model + ": "))
			sb.WriteString(assistantMsgStyle.Render(msg.content))
		case "system":
			sb.WriteString(systemMsgStyle.Render(msg.content))
		case "error":
			sb.WriteString(errorMsgStyle.Render("Error: " + msg.content))
		}
		sb.WriteString("\n\n")
	}

	// Show in-progress streaming text.
	if streamBuf != "" {
		sb.WriteString(render.AssistantStyle.Render(model + ": "))
		sb.WriteString(assistantMsgStyle.Render(streamBuf))
		sb.WriteString("\n")
	}

	return sb.String()
}
