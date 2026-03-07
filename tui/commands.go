package tui

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dev-ben-c/localfreshllm/backend"
	"github.com/dev-ben-c/localfreshllm/service"
)

// chatEvent is an internal event sent over channels from the streaming goroutine.
type chatEvent struct {
	typ      string // "token", "tool_call", "done", "error"
	token    string
	toolName string
	text     string
	newMsgs  []backend.Message
	err      error
}

// Tea messages.
type tokenMsg string

type toolCallMsg struct {
	name string
}

type doneMsg struct {
	text    string
	newMsgs []backend.Message
}

type errorMsg struct {
	err error
}

// startChat launches a goroutine that runs the chat service and pushes events to ch.
func startChat(cfg Config, messages []backend.Message, ch chan chatEvent) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT)
		defer cancel()

		location := ""
		if cfg.UserConfig != nil {
			location = cfg.UserConfig.Location
		}

		req := service.ChatRequest{
			Model:        cfg.Model,
			Messages:     messages,
			SystemPrompt: cfg.SystemPrompt,
			Location:     location,
			EnableTools:  cfg.EnableTools,
		}

		emit := func(ev service.ChatEvent) {
			switch ev.Type {
			case "token":
				ch <- chatEvent{typ: "token", token: ev.Token}
			case "tool_call":
				ch <- chatEvent{typ: "tool_call", toolName: ev.ToolName}
			case "done":
				ch <- chatEvent{typ: "done", text: ev.Text, newMsgs: ev.Messages}
			case "error":
				ch <- chatEvent{typ: "error", text: ev.Text}
			}
		}

		_, _, err := cfg.ChatService.Chat(ctx, cfg.Backend, req, emit)
		if err != nil && ctx.Err() == nil {
			ch <- chatEvent{typ: "error", err: err}
		}
		close(ch)
		return nil
	}
}

// waitForEvent reads the next event from the channel and converts it to a tea.Msg.
func waitForEvent(ch chan chatEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		switch ev.typ {
		case "token":
			return tokenMsg(ev.token)
		case "tool_call":
			return toolCallMsg{name: ev.toolName}
		case "done":
			return doneMsg{text: ev.text, newMsgs: ev.newMsgs}
		case "error":
			if ev.err != nil {
				return errorMsg{err: ev.err}
			}
			return errorMsg{err: os.ErrClosed}
		}
		return nil
	}
}
