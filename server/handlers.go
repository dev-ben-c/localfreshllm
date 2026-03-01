package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rabidclock/localfreshllm/backend"
	"github.com/rabidclock/localfreshllm/service"
	"github.com/rabidclock/localfreshllm/session"
	"github.com/rabidclock/localfreshllm/systemprompt"
)

// chatRequest is the JSON body for POST /v1/chat.
type chatRequest struct {
	Message      string `json:"message"`
	SessionID    string `json:"session_id,omitempty"`
	Model        string `json:"model,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	Persona      string `json:"persona,omitempty"`
	EnableTools  *bool  `json:"enable_tools,omitempty"`
}

// registerRequest is the JSON body for POST /v1/devices/register.
type registerRequest struct {
	Name            string `json:"name"`
	RegistrationKey string `json:"registration_key"`
}

// deviceUpdateRequest is the JSON body for PUT /v1/devices/me.
type deviceUpdateRequest struct {
	Name     string `json:"name,omitempty"`
	Model    string `json:"model,omitempty"`
	Location string `json:"location,omitempty"`
	Persona  string `json:"persona,omitempty"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	dev := DeviceFromContext(r.Context())
	if dev == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, `{"error":"message is required"}`, http.StatusBadRequest)
		return
	}

	// Resolve model: request > device profile > default.
	model := "qwen3:32b"
	if dev.Model != "" {
		model = dev.Model
	}
	if req.Model != "" {
		model = req.Model
	}

	// Resolve system prompt.
	sysPrompt := systemprompt.Get(req.SystemPrompt, req.Persona)
	if sysPrompt == "" && dev.Persona != "" {
		sysPrompt = systemprompt.Get("", dev.Persona)
	}

	// Resolve location.
	location := dev.Location

	// Resolve tools.
	enableTools := true
	if req.EnableTools != nil {
		enableTools = *req.EnableTools
	}

	// Load or create session.
	store := s.devices.SessionStore(dev.ID)
	var sess *session.Session
	if req.SessionID != "" {
		var err error
		sess, err = store.FindByPrefix(req.SessionID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"session not found: %s"}`, err.Error()), http.StatusNotFound)
			return
		}
	}
	if sess == nil {
		sess = session.NewSession(uuid.New().String()[:8], model)
	}

	sess.AddMessage("user", req.Message)

	// Set up SSE.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	b := backend.ForModel(model)
	if err := b.Validate(); err != nil {
		// Fall back to first Ollama model.
		ollama := backend.NewOllama()
		models, listErr := ollama.ListModels(r.Context())
		if listErr != nil || len(models) == 0 {
			WriteEvent(w, "error", fmt.Sprintf(`{"text":"backend unavailable: %s"}`, err.Error()))
			return
		}
		model = models[0]
		b = ollama
	}

	chatReq := service.ChatRequest{
		Model:        model,
		Messages:     sess.Messages,
		SystemPrompt: sysPrompt,
		Location:     location,
		EnableTools:  enableTools,
	}

	ctx := r.Context()

	emit := func(ev service.ChatEvent) {
		switch ev.Type {
		case "token":
			data, _ := json.Marshal(map[string]string{"text": ev.Token})
			WriteEvent(w, "token", string(data))
		case "tool_call":
			data, _ := json.Marshal(map[string]string{"name": ev.ToolName, "id": ev.ToolID})
			WriteEvent(w, "tool_call", string(data))
		case "tool_result":
			data, _ := json.Marshal(map[string]string{"name": ev.ToolName, "id": ev.ToolID, "text": ev.Text})
			WriteEvent(w, "tool_result", string(data))
		case "error":
			data, _ := json.Marshal(map[string]string{"text": ev.Text})
			WriteEvent(w, "error", string(data))
		case "done":
			// Save session before sending done event.
			for _, m := range ev.Messages {
				sess.Messages = append(sess.Messages, m)
			}
			if ev.Text != "" {
				sess.AddMessage("assistant", ev.Text)
			}
			if saveErr := store.Save(sess); saveErr != nil {
				data, _ := json.Marshal(map[string]string{"text": fmt.Sprintf("failed to save session: %s", saveErr.Error())})
				WriteEvent(w, "error", string(data))
			}
			data, _ := json.Marshal(map[string]string{"text": ev.Text, "session_id": sess.ID})
			WriteEvent(w, "done", string(data))
		}
	}

	response, _, err := s.chatService.Chat(ctx, b, chatReq, emit)
	_ = response
	if err != nil && ctx.Err() == nil {
		data, _ := json.Marshal(map[string]string{"text": err.Error()})
		WriteEvent(w, "error", string(data))
	}

	// Update last seen.
	dev.LastSeenAt = time.Now()
	s.devices.Update(dev)
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	models := service.ListModels(r.Context())
	data, _ := json.Marshal(map[string][]string{"models": models})
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	profile, err := s.devices.Register(req.Name, req.RegistrationKey, s.masterKey)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "invalid registration key") {
			status = http.StatusForbidden
		}
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), status)
		return
	}

	data, _ := json.Marshal(profile)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(data)
}

func (s *Server) handleDeviceMe(w http.ResponseWriter, r *http.Request) {
	dev := DeviceFromContext(r.Context())
	if dev == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		data, _ := json.Marshal(dev)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)

	case http.MethodPut:
		var req deviceUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Name != "" {
			dev.Name = req.Name
		}
		if req.Model != "" {
			dev.Model = req.Model
		}
		if req.Location != "" {
			dev.Location = req.Location
		}
		if req.Persona != "" {
			dev.Persona = req.Persona
		}
		if err := s.devices.Update(dev); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"update failed: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		data, _ := json.Marshal(dev)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	dev := DeviceFromContext(r.Context())
	if dev == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	store := s.devices.SessionStore(dev.ID)

	switch r.Method {
	case http.MethodGet:
		sessions, err := store.List()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"list sessions: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		type sessionSummary struct {
			ID        string    `json:"id"`
			Model     string    `json:"model"`
			UpdatedAt time.Time `json:"updated_at"`
			Messages  int       `json:"messages"`
			Preview   string    `json:"preview"`
		}
		var summaries []sessionSummary
		for _, sess := range sessions {
			summaries = append(summaries, sessionSummary{
				ID:        sess.ID,
				Model:     sess.Model,
				UpdatedAt: sess.UpdatedAt,
				Messages:  len(sess.Messages),
				Preview:   sess.Preview(),
			})
		}
		data, _ := json.Marshal(map[string]any{"sessions": summaries})
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	dev := DeviceFromContext(r.Context())
	if dev == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Extract session ID from path: /v1/sessions/{id}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/sessions/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, `{"error":"session ID required"}`, http.StatusBadRequest)
		return
	}
	sessionID := parts[0]

	store := s.devices.SessionStore(dev.ID)

	switch r.Method {
	case http.MethodGet:
		sess, err := store.FindByPrefix(sessionID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
			return
		}
		data, _ := json.Marshal(sess)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)

	case http.MethodDelete:
		sess, err := store.FindByPrefix(sessionID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
			return
		}
		if err := store.Delete(sess.ID); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"delete failed: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// healthCheck returns a simple status response.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	models := service.ListModels(ctx)
	data, _ := json.Marshal(map[string]any{
		"status": "ok",
		"models": len(models),
	})
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
