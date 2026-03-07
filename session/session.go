package session

import (
	"time"

	"github.com/dev-ben-c/localfreshllm/backend"
)

// Session represents a conversation with metadata.
type Session struct {
	ID        string            `json:"id"`
	Model     string            `json:"model"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Messages  []backend.Message `json:"messages"`
}

// NewSession creates a new session with the given ID and model.
func NewSession(id, model string) *Session {
	now := time.Now()
	return &Session{
		ID:        id,
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []backend.Message{},
	}
}

// AddMessage appends a message and updates the timestamp.
func (s *Session) AddMessage(role, content string) {
	s.Messages = append(s.Messages, backend.Message{Role: role, Content: content})
	s.UpdatedAt = time.Now()
}

// Preview returns a short preview of the first user message.
func (s *Session) Preview() string {
	for _, m := range s.Messages {
		if m.Role == "user" {
			if len(m.Content) > 80 {
				return m.Content[:80] + "..."
			}
			return m.Content
		}
	}
	return "(empty)"
}
