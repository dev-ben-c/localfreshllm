package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dev-ben-c/localfreshllm/backend"
	"github.com/dev-ben-c/localfreshllm/device"
	"github.com/dev-ben-c/localfreshllm/service"
	"github.com/dev-ben-c/localfreshllm/session"
	"github.com/dev-ben-c/localfreshllm/shell"
	"github.com/dev-ben-c/localfreshllm/systemprompt"
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

// parseChatRequest decodes and validates the chat request body.
func parseChatRequest(r *http.Request) (*chatRequest, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxJSONBodySize)
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Message) == "" {
		return nil, fmt.Errorf("message is required")
	}
	return &req, nil
}

// chatConfig holds resolved configuration for a chat request.
type chatConfig struct {
	Model       string
	SysPrompt   string
	Location    string
	EnableTools bool
}

// resolveChatConfig resolves the model, system prompt, location, and tools
// from request and device profile defaults.
func resolveChatConfig(req *chatRequest, dev *device.Profile) chatConfig {
	cfg := chatConfig{
		Model:       "qwen3:32b",
		EnableTools: true,
	}

	if dev.Model != "" {
		cfg.Model = dev.Model
	}
	if req.Model != "" {
		cfg.Model = req.Model
	}

	cfg.SysPrompt = systemprompt.Get(req.SystemPrompt, req.Persona)
	if cfg.SysPrompt == "" && dev.Persona != "" {
		cfg.SysPrompt = systemprompt.Get("", dev.Persona)
	}

	cfg.Location = dev.Location

	if req.EnableTools != nil {
		cfg.EnableTools = *req.EnableTools
	}

	return cfg
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	dev := DeviceFromContext(r.Context())
	if dev == nil {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	req, err := parseChatRequest(r)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			jsonError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		if err.Error() == "message is required" {
			jsonError(w, http.StatusBadRequest, "message is required")
			return
		}
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cfg := resolveChatConfig(req, dev)

	// Load or create session.
	store := s.devices.SessionStore(dev.ID)
	var sess *session.Session
	if req.SessionID != "" {
		sess, err = store.FindByPrefix(req.SessionID)
		if err != nil {
			jsonError(w, http.StatusNotFound, "session not found")
			log.Printf("session lookup failed for prefix %q: %v", req.SessionID, err)
			return
		}
	}
	if sess == nil {
		sess = session.NewSession(uuid.New().String()[:8], cfg.Model)
	}

	sess.AddMessage("user", req.Message)

	// Set up SSE.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	b := backend.ForModel(cfg.Model)
	if err := b.Validate(); err != nil {
		// Fall back to first Ollama model.
		ollama := backend.NewOllama()
		models, listErr := ollama.ListModels(r.Context())
		if listErr != nil || len(models) == 0 {
			WriteEvent(w, "error", `{"text":"backend unavailable"}`)
			log.Printf("backend validation failed: %v", err)
			return
		}
		cfg.Model = models[0]
		b = ollama
	}

	chatReq := service.ChatRequest{
		Model:        cfg.Model,
		Messages:     sess.Messages,
		SystemPrompt: cfg.SysPrompt,
		Location:     cfg.Location,
		EnableTools:  cfg.EnableTools,
		SudoPassword: s.sudoStore.Get(dev.ID),
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
				WriteEvent(w, "error", `{"text":"failed to save session"}`)
				log.Printf("save session %s: %v", sess.ID, saveErr)
			}
			data, _ := json.Marshal(map[string]string{"text": ev.Text, "session_id": sess.ID})
			WriteEvent(w, "done", string(data))
		}
	}

	response, _, err := s.chatService.Chat(ctx, b, chatReq, emit)
	_ = response
	if err != nil && ctx.Err() == nil {
		WriteEvent(w, "error", `{"text":"chat request failed"}`)
		log.Printf("chat error: %v", err)
	}

	// Update last seen.
	dev.LastSeenAt = time.Now()
	s.devices.Update(dev)
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	models := service.ListModels(r.Context())
	jsonOK(w, map[string][]string{"models": models})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodySize)
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			jsonError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	profile, err := s.devices.Register(req.Name, req.RegistrationKey, s.masterKey)
	if err != nil {
		if strings.Contains(err.Error(), "invalid registration key") {
			jsonError(w, http.StatusForbidden, "invalid registration key")
			return
		}
		jsonError(w, http.StatusBadRequest, "registration failed")
		log.Printf("device registration failed: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(profile)
}

func (s *Server) handleDeviceMe(w http.ResponseWriter, r *http.Request) {
	dev := DeviceFromContext(r.Context())
	if dev == nil {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	switch r.Method {
	case http.MethodGet:
		jsonOK(w, dev)

	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodySize)
		var req deviceUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				jsonError(w, http.StatusRequestEntityTooLarge, "request body too large")
				return
			}
			jsonError(w, http.StatusBadRequest, "invalid request body")
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
			jsonError(w, http.StatusInternalServerError, "update failed")
			log.Printf("device update failed for %s: %v", dev.ID, err)
			return
		}
		jsonOK(w, dev)

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	dev := DeviceFromContext(r.Context())
	if dev == nil {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	store := s.devices.SessionStore(dev.ID)

	switch r.Method {
	case http.MethodGet:
		sessions, err := store.List()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to list sessions")
			log.Printf("list sessions for device %s: %v", dev.ID, err)
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
		jsonOK(w, map[string]any{"sessions": summaries})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	dev := DeviceFromContext(r.Context())
	if dev == nil {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Extract session ID from path: /v1/sessions/{id}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/sessions/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		jsonError(w, http.StatusBadRequest, "session ID required")
		return
	}
	sessionID := parts[0]

	store := s.devices.SessionStore(dev.ID)

	switch r.Method {
	case http.MethodGet:
		sess, err := store.FindByPrefix(sessionID)
		if err != nil {
			jsonError(w, http.StatusNotFound, "session not found")
			return
		}
		jsonOK(w, sess)

	case http.MethodDelete:
		sess, err := store.FindByPrefix(sessionID)
		if err != nil {
			jsonError(w, http.StatusNotFound, "session not found")
			return
		}
		if err := store.Delete(sess.ID); err != nil {
			jsonError(w, http.StatusInternalServerError, "delete failed")
			log.Printf("delete session %s: %v", sess.ID, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// sudoAuthRequest is the JSON body for POST /v1/sudo/auth.
type sudoAuthRequest struct {
	Password string `json:"password"`
}

func (s *Server) handleSudoAuth(w http.ResponseWriter, r *http.Request) {
	dev := DeviceFromContext(r.Context())
	if dev == nil {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	switch r.Method {
	case http.MethodPost:
		if !shell.IsEnabled() {
			jsonError(w, http.StatusBadRequest, "shell execution is not enabled")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodySize)
		var req sudoAuthRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Password == "" {
			jsonError(w, http.StatusBadRequest, "password is required")
			return
		}
		if err := shell.ValidatePassword(req.Password); err != nil {
			jsonError(w, http.StatusForbidden, "invalid sudo password")
			return
		}
		s.sudoStore.Set(dev.ID, req.Password)
		jsonOK(w, map[string]any{"status": "authenticated", "ttl_minutes": 15})

	case http.MethodDelete:
		s.sudoStore.Clear(dev.ID)
		w.WriteHeader(http.StatusNoContent)

	case http.MethodGet:
		active := s.sudoStore.Get(dev.ID) != ""
		jsonOK(w, map[string]bool{"active": active})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// healthCheck returns a simple status response.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	models := service.ListModels(ctx)
	jsonOK(w, map[string]any{
		"status": "ok",
		"models": len(models),
	})
}
