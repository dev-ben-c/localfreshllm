package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rabidclock/localfreshllm/audio/capture"
	"github.com/rabidclock/localfreshllm/audio/playback"
	"github.com/rabidclock/localfreshllm/backend"
	"github.com/rabidclock/localfreshllm/render"
	"github.com/rabidclock/localfreshllm/service"
	"github.com/rabidclock/localfreshllm/session"
)

type state int

const (
	stateIdle state = iota
	stateStreaming
	stateModelPick
)

// Config holds dependencies injected into the TUI model.
type Config struct {
	Backend      backend.Backend
	ChatService  *service.ChatService
	Store        *session.Store
	Session      *session.Session
	UserConfig   *session.Config
	Model        string
	SystemPrompt string
	EnableTools  bool
	RenderMD     bool
	IsClient     bool
	PiperModel   string
	PiperSpeaker string
	WhisperURL   string
}

// Model is the top-level Bubble Tea model.
type Model struct {
	cfg Config

	state    state
	viewport viewport.Model
	input    textinput.Model
	mascot   MascotModel

	messages  []chatMessage
	streamBuf *strings.Builder
	streamCh  chan chatEvent

	width  int
	height int

	history []string
	histIdx int
	histTmp string

	ttsEnabled bool
	player     *playback.Player

	voiceMode bool
	listening bool
	listener  *capture.Listener

	timers       []Timer
	timerTicking bool

	err error
}

type chatMessage struct {
	role    string
	content string
}

const (
	headerHeight = 8
	inputHeight  = 3
)

// New creates a new TUI model with the given config.
func New(cfg Config) Model {
	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.Focus()
	ti.CharLimit = 0
	ti.Width = 80
	ti.Prompt = render.UserStyle.Render("You: ")

	vp := viewport.New(80, 20)
	vp.SetContent("")

	m := Model{
		cfg:       cfg,
		state:     stateIdle,
		input:     ti,
		viewport:  vp,
		mascot:    NewMascotModel(),
		streamBuf: &strings.Builder{},
		history:   loadHistory(),
		histIdx:   -1,
		player:   &playback.Player{},
		listener: &capture.Listener{},
	}

	// Restore session messages into chat display.
	if cfg.Session != nil {
		for _, msg := range cfg.Session.Messages {
			if msg.Role == "user" || msg.Role == "assistant" {
				m.messages = append(m.messages, chatMessage{role: msg.Role, content: msg.Content})
			}
		}
	}

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, mascotTick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - headerHeight - inputHeight
		m.input.Width = msg.Width - lipgloss.Width(m.input.Prompt) - 1
		m.rebuildViewport()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case mascotTickMsg:
		var cmd tea.Cmd
		m.mascot, cmd = m.mascot.Update(msg)
		return m, cmd

	case tokenMsg:
		m.streamBuf.WriteString(string(msg))
		m.rebuildViewport()
		return m, waitForEvent(m.streamCh)

	case toolCallMsg:
		m.messages = append(m.messages, chatMessage{
			role:    "system",
			content: "[tool: " + msg.name + "]",
		})
		m.rebuildViewport()
		return m, waitForEvent(m.streamCh)

	case doneMsg:
		text := m.streamBuf.String()
		if m.cfg.RenderMD && text != "" {
			text = strings.TrimSpace(render.RenderMarkdown(text))
		}
		if text != "" {
			m.messages = append(m.messages, chatMessage{role: "assistant", content: text})
		}

		// Save intermediate messages to session.
		if m.cfg.Session != nil {
			for _, nm := range msg.newMsgs {
				m.cfg.Session.Messages = append(m.cfg.Session.Messages, nm)
			}
			if m.streamBuf.String() != "" {
				m.cfg.Session.AddMessage("assistant", m.streamBuf.String())
			}
			if m.cfg.Store != nil {
				m.cfg.Store.Save(m.cfg.Session)
			}
		}

		// Trigger TTS if enabled and there's text to speak.
		rawText := m.streamBuf.String()

		m.streamBuf.Reset()
		m.state = stateIdle
		m.mascot.state = mascotIdle
		m.input.Focus()
		m.rebuildViewport()

		var batchCmds []tea.Cmd
		batchCmds = append(batchCmds, textinput.Blink)
		if m.ttsEnabled && rawText != "" {
			batchCmds = append(batchCmds, playTTS(m.cfg, m.player, rawText))
		}
		// Resume listening after response if voice mode is active.
		if m.voiceMode && !m.listening {
			m.listening = true
			batchCmds = append(batchCmds, listenForSegment(m.listener))
		}
		return m, tea.Batch(batchCmds...)

	case errorMsg:
		m.streamBuf.Reset()
		m.state = stateIdle
		m.mascot.state = mascotIdle
		m.err = msg.err
		m.messages = append(m.messages, chatMessage{
			role:    "error",
			content: msg.err.Error(),
		})
		m.input.Focus()
		m.rebuildViewport()
		return m, textinput.Blink

	case timerTickMsg:
		expired := checkExpired(&m.timers)
		if len(expired) > 0 {
			// Fire expiry handling as a separate message.
			return m, func() tea.Msg { return timerExpiredMsg{names: expired} }
		}
		m.rebuildViewport()
		if len(m.timers) > 0 {
			return m, timerTick()
		}
		m.timerTicking = false
		return m, nil

	case timerExpiredMsg:
		for _, name := range msg.names {
			m.messages = append(m.messages, chatMessage{
				role:    "system",
				content: fmt.Sprintf("Timer expired: %s", name),
			})
		}
		m.rebuildViewport()
		if m.ttsEnabled && len(msg.names) > 0 {
			label := msg.names[0]
			if len(msg.names) > 1 {
				label = fmt.Sprintf("%d timers", len(msg.names))
			}
			return m, playTTS(m.cfg, m.player, label+" timer is done")
		}
		return m, nil

	case voiceSegmentMsg:
		if msg.err != nil {
			m.listening = false
			m.messages = append(m.messages, chatMessage{
				role:    "error",
				content: "Listener: " + msg.err.Error(),
			})
			m.rebuildViewport()
			return m, nil
		}
		if len(msg.pcm) == 0 {
			// No data — keep listening.
			if m.voiceMode {
				return m, listenForSegment(m.listener)
			}
			return m, nil
		}
		// Got a speech segment — transcribe it.
		m.messages = append(m.messages, chatMessage{role: "system", content: "Transcribing..."})
		m.rebuildViewport()
		return m, transcribeVoiceSegment(m.cfg, msg.pcm)

	case voiceTranscribedMsg:
		// Remove "Transcribing..." message.
		if len(m.messages) > 0 && m.messages[len(m.messages)-1].content == "Transcribing..." {
			m.messages = m.messages[:len(m.messages)-1]
		}
		if msg.err != nil {
			m.messages = append(m.messages, chatMessage{
				role:    "error",
				content: "Transcribe: " + msg.err.Error(),
			})
			m.rebuildViewport()
			// Resume listening.
			if m.voiceMode {
				return m, listenForSegment(m.listener)
			}
			return m, nil
		}

		text := strings.TrimSpace(msg.text)
		if text == "" {
			// Empty transcription — keep listening.
			if m.voiceMode {
				return m, listenForSegment(m.listener)
			}
			return m, nil
		}

		// Check for wake word.
		afterWake, hasWake := extractAfterWakeWord(text)
		if !hasWake {
			// No wake word — ignore and keep listening.
			if m.voiceMode {
				return m, listenForSegment(m.listener)
			}
			return m, nil
		}

		if afterWake == "" {
			// Just the wake word by itself — acknowledge and keep listening.
			m.messages = append(m.messages, chatMessage{role: "system", content: "Listening..."})
			m.rebuildViewport()
			if m.voiceMode {
				return m, listenForSegment(m.listener)
			}
			return m, nil
		}

		// Wake word + text — auto-submit.
		m.input.SetValue(afterWake)
		return m.handleSubmit()

	case audioPlayDoneMsg:
		if msg.err != nil {
			m.messages = append(m.messages, chatMessage{
				role:    "error",
				content: "TTS: " + msg.err.Error(),
			})
			m.rebuildViewport()
		}
		return m, nil

	case slashResultMsg:
		if msg.quit {
			return m, tea.Quit
		}
		if msg.ttsToggle {
			m.ttsEnabled = !m.ttsEnabled
			status := "enabled"
			if !m.ttsEnabled {
				status = "disabled"
			}
			m.messages = append(m.messages, chatMessage{role: "system", content: "TTS " + status})
		}
		if msg.voiceToggle {
			return m.toggleVoiceMode()
		}
		// Timer actions.
		if msg.timerAdd != nil {
			if len(m.timers) >= maxTimers {
				m.messages = append(m.messages, chatMessage{role: "system", content: fmt.Sprintf("Maximum %d timers reached. Cancel one first.", maxTimers)})
			} else {
				t := *msg.timerAdd
				t.Deadline = time.Now().Add(t.Duration)
				m.timers = append(m.timers, t)
				m.messages = append(m.messages, chatMessage{
					role:    "system",
					content: fmt.Sprintf("Timer started: %s (%s)", t.Name, formatRemaining(t.Duration)),
				})
				if !m.timerTicking {
					m.timerTicking = true
					m.rebuildViewport()
					return m, timerTick()
				}
			}
		}
		if msg.timerCancel > 0 {
			idx := msg.timerCancel - 1
			if idx >= len(m.timers) {
				m.messages = append(m.messages, chatMessage{role: "system", content: fmt.Sprintf("No timer #%d", msg.timerCancel)})
			} else {
				name := m.timers[idx].Name
				m.timers = append(m.timers[:idx], m.timers[idx+1:]...)
				m.messages = append(m.messages, chatMessage{role: "system", content: fmt.Sprintf("Cancelled timer: %s", name)})
			}
		}
		if msg.timerClear {
			n := len(m.timers)
			m.timers = nil
			m.messages = append(m.messages, chatMessage{role: "system", content: fmt.Sprintf("Cleared %d timer(s)", n)})
		}
		if msg.info != "" {
			// Special sentinel for timer list — we handle it here since Model owns the timers.
			if msg.info == "_timer_list_" {
				if len(m.timers) == 0 {
					m.messages = append(m.messages, chatMessage{role: "system", content: "No active timers. Use /timer <duration> [name] to start one."})
				} else {
					var sb strings.Builder
					sb.WriteString(fmt.Sprintf("Active timers (%d/%d):\n", len(m.timers), maxTimers))
					for i, t := range m.timers {
						sb.WriteString(fmt.Sprintf("  [%d] %s — %s remaining\n", i+1, t.Name, formatRemaining(t.Remaining())))
					}
					m.messages = append(m.messages, chatMessage{role: "system", content: sb.String()})
				}
			} else {
				m.messages = append(m.messages, chatMessage{role: "system", content: msg.info})
			}
		}
		if msg.modelPick {
			m.state = stateModelPick
		}
		m.rebuildViewport()
		return m, nil
	}

	// Update sub-components.
	if m.state == stateIdle || m.state == stateModelPick {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit

	case tea.KeyUp:
		if m.state == stateIdle && len(m.history) > 0 {
			if m.histIdx == -1 {
				m.histTmp = m.input.Value()
				m.histIdx = len(m.history) - 1
			} else if m.histIdx > 0 {
				m.histIdx--
			}
			m.input.SetValue(m.history[m.histIdx])
			m.input.CursorEnd()
			return m, nil
		}

	case tea.KeyDown:
		if m.state == stateIdle && m.histIdx >= 0 {
			m.histIdx++
			if m.histIdx >= len(m.history) {
				m.histIdx = -1
				m.input.SetValue(m.histTmp)
			} else {
				m.input.SetValue(m.history[m.histIdx])
			}
			m.input.CursorEnd()
			return m, nil
		}

	case tea.KeyPgUp:
		m.viewport.HalfViewUp()
		return m, nil

	case tea.KeyPgDown:
		m.viewport.HalfViewDown()
		return m, nil

	case tea.KeyCtrlAt, tea.KeyF5:
		// Toggle voice mode on/off.
		return m.toggleVoiceMode()

	case tea.KeyEnter:
		return m.handleSubmit()
	}

	// Forward to text input.
	if m.state == stateIdle || m.state == stateModelPick {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleSubmit() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.input.Value())
	if input == "" {
		return m, nil
	}

	m.input.SetValue("")
	m.histIdx = -1
	m.err = nil

	// Model picker mode.
	if m.state == stateModelPick {
		result := handleModelPickInput(input, &m.cfg)
		m.state = stateIdle
		if result.info != "" {
			m.messages = append(m.messages, chatMessage{role: "system", content: result.info})
		}
		m.rebuildViewport()
		return m, nil
	}

	// Slash commands.
	if strings.HasPrefix(input, "/") {
		result := handleSlash(input, &m.cfg)
		msg := slashResultMsg(result)
		return m.Update(msg)
	}

	// Save to input history.
	m.history = append(m.history, input)
	saveHistory(m.history)

	// Add user message.
	m.messages = append(m.messages, chatMessage{role: "user", content: input})
	if m.cfg.Session != nil {
		m.cfg.Session.AddMessage("user", input)
	}

	// Start streaming. Pause voice listening during response.
	m.state = stateStreaming
	m.mascot.state = mascotThinking
	m.listening = false
	m.input.Blur()
	m.rebuildViewport()

	ch := make(chan chatEvent, 64)
	m.streamCh = ch

	var messages []backend.Message
	if m.cfg.IsClient {
		// Client mode: send only user messages, server has session.
		messages = []backend.Message{{Role: "user", Content: input}}
	} else if m.cfg.Session != nil {
		messages = m.cfg.Session.Messages
	}

	return m, tea.Batch(
		startChat(m.cfg, messages, ch),
		waitForEvent(ch),
	)
}

func (m *Model) toggleVoiceMode() (tea.Model, tea.Cmd) {
	m.voiceMode = !m.voiceMode
	if m.voiceMode {
		m.messages = append(m.messages, chatMessage{
			role:    "system",
			content: "Voice mode enabled — say \"lemon\" followed by your message",
		})
		// Create a fresh listener each time (Stop kills the subprocess).
		m.listener = &capture.Listener{}
		m.listening = true
		m.rebuildViewport()
		return m, startAndListen(m.listener)
	}
	// Disable voice mode.
	m.listening = false
	m.listener.Stop()
	m.messages = append(m.messages, chatMessage{role: "system", content: "Voice mode disabled"})
	m.rebuildViewport()
	return m, nil
}

func (m *Model) rebuildViewport() {
	content := buildContent(m.messages, m.streamBuf.String(), m.cfg.Model, m.width)
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	// Header: mascot + status.
	mascotView := m.mascot.View()
	statusLine := render.ModelStyle.Render(m.cfg.Model)
	if m.state == stateStreaming {
		statusLine += render.DimStyle.Render("  streaming...")
	}
	if m.cfg.IsClient {
		statusLine += render.DimStyle.Render("  (remote)")
	}
	toolsStatus := "on"
	if !m.cfg.EnableTools {
		toolsStatus = "off"
	}
	ttsStatus := "off"
	if m.ttsEnabled {
		ttsStatus = "on"
	}
	voiceStatus := "off"
	if m.voiceMode {
		voiceStatus = "on"
	}
	statusLine += "\n" + render.DimStyle.Render("tools: "+toolsStatus+"  tts: "+ttsStatus+"  voice: "+voiceStatus)
	if ts := renderTimerStatus(m.timers); ts != "" {
		statusLine += "\n" + render.TimerStyle.Render(ts)
	}
	statusLine += "\n" + render.DimStyle.Render("/help for commands")

	mascotWidth := lipgloss.Width(mascotView)
	statusStyle := lipgloss.NewStyle().
		Width(m.width - mascotWidth - 2).
		PaddingLeft(2)

	header := lipgloss.JoinHorizontal(lipgloss.Top, mascotView, statusStyle.Render(statusLine))

	headerStyle := lipgloss.NewStyle().
		Height(headerHeight).
		MaxHeight(headerHeight)

	// Dividers.
	divider := render.DimStyle.Render(strings.Repeat("─", m.width))

	// Input area.
	inputView := m.input.View()
	if m.voiceMode && m.listening && m.state != stateStreaming {
		inputView = render.DimStyle.Render("  Listening for \"lemon\"... (Ctrl+Space to stop)")
	} else if m.state == stateStreaming {
		inputView = render.DimStyle.Render("  waiting for response...")
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		headerStyle.Render(header),
		divider,
		m.viewport.View(),
		divider,
		inputView,
	)
}
