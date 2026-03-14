package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"

	"github.com/dev-ben-c/localfreshllm/render"
)

var (
	userMsgStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	assistantMsgStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	systemMsgStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errorMsgStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
)

// wrapText wraps plain text at word boundaries so no line exceeds width columns.
// It preserves existing newlines within the text.
func wrapText(s string, width int) string {
	if width <= 0 || s == "" {
		return s
	}
	return wordwrap.String(s, width)
}

// buildContent renders the chat messages plus any in-progress stream into a single string.
func buildContent(messages []chatMessage, streamBuf string, model string, width int) string {
	if width <= 0 {
		width = 80
	}

	var sb strings.Builder

	for _, msg := range messages {
		switch msg.role {
		case "user":
			label := "You: "
			wrapped := wrapText(msg.content, width-lipgloss.Width(label))
			sb.WriteString(render.UserStyle.Render(label))
			sb.WriteString(userMsgStyle.Render(wrapped))
		case "assistant":
			label := model + ": "
			wrapped := wrapText(msg.content, width-lipgloss.Width(label))
			sb.WriteString(render.AssistantStyle.Render(label))
			sb.WriteString(assistantMsgStyle.Render(wrapped))
		case "system":
			wrapped := wrapText(msg.content, width)
			sb.WriteString(systemMsgStyle.Render(wrapped))
		case "error":
			wrapped := wrapText("Error: "+msg.content, width)
			sb.WriteString(errorMsgStyle.Render(wrapped))
		}
		sb.WriteString("\n\n")
	}

	// Show in-progress streaming text.
	if streamBuf != "" {
		label := model + ": "
		wrapped := wrapText(streamBuf, width-lipgloss.Width(label))
		sb.WriteString(render.AssistantStyle.Render(label))
		sb.WriteString(assistantMsgStyle.Render(wrapped))
		sb.WriteString("\n")
	}

	return sb.String()
}
