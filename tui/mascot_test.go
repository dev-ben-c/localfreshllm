package tui

import (
	"strings"
	"testing"
	"time"
)

func TestNewMascotModel(t *testing.T) {
	m := NewMascotModel()
	if m.state != mascotIdle {
		t.Errorf("expected idle state, got %d", m.state)
	}
	if m.frame != 0 {
		t.Errorf("expected frame 0, got %d", m.frame)
	}
}

func TestMascotModel_ThinkingFrameAdvance(t *testing.T) {
	m := NewMascotModel()
	m.state = mascotThinking

	// Advance through frames.
	for i := 0; i < 6; i++ {
		expected := (i + 1) % len(thinkingDots)
		var cmd interface{}
		m, cmd = m.Update(mascotTickMsg(time.Now()))
		_ = cmd
		if m.frame != expected {
			t.Errorf("tick %d: expected frame %d, got %d", i+1, expected, m.frame)
		}
	}
}

func TestMascotModel_IdleNoFrameAdvance(t *testing.T) {
	m := NewMascotModel()
	m.state = mascotIdle

	m, _ = m.Update(mascotTickMsg(time.Now()))
	if m.frame != 0 {
		t.Errorf("idle state should not advance frame, got %d", m.frame)
	}
}

func TestMascotModel_ViewIdle(t *testing.T) {
	m := NewMascotModel()
	m.state = mascotIdle

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
	// Should contain braille characters.
	if !strings.Contains(view, "⣠⠤⡀") {
		t.Error("expected leaf braille pattern in idle view")
	}
}

func TestMascotModel_ViewThinking(t *testing.T) {
	m := NewMascotModel()
	m.state = mascotThinking

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
	// Should contain the leaf.
	if !strings.Contains(view, "⣠⠤⡀") {
		t.Error("expected leaf braille pattern in thinking view")
	}
}

func TestMascotModel_ViewSpeaking(t *testing.T) {
	m := NewMascotModel()
	m.state = mascotSpeaking

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
	if !strings.Contains(view, "⣠⠤⡀") {
		t.Error("expected leaf braille pattern in speaking view")
	}
}

func TestMascotModel_ViewChangesPerState(t *testing.T) {
	m := NewMascotModel()

	m.state = mascotIdle
	idleView := m.View()

	m.state = mascotThinking
	thinkingView := m.View()

	m.state = mascotSpeaking
	speakingView := m.View()

	// Views should differ between states.
	if idleView == thinkingView {
		t.Error("idle and thinking views should differ")
	}
	if idleView == speakingView {
		t.Error("idle and speaking views should differ")
	}
}

func TestThinkingDots(t *testing.T) {
	if len(thinkingDots) != 3 {
		t.Errorf("expected 3 thinking dot frames, got %d", len(thinkingDots))
	}
	for i, d := range thinkingDots {
		if d == "" {
			t.Errorf("thinking dot frame %d is empty", i)
		}
	}
}
