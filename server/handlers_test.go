package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rabidclock/localfreshllm/audio"
	"github.com/rabidclock/localfreshllm/device"
	"github.com/rabidclock/localfreshllm/session"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataDir)
	return &Server{
		masterKey: "test-master-key",
		devices:   device.NewStore(),
	}
}

// TestHandleHealth verifies the health endpoint returns status and models count.
func TestHandleHealth(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", result["status"])
	}
	if _, ok := result["models"]; !ok {
		t.Errorf("expected 'models' field in response")
	}
}

// TestHandleRegister_Valid tests successful device registration.
func TestHandleRegister_Valid(t *testing.T) {
	srv := newTestServer(t)

	body, _ := json.Marshal(registerRequest{
		Name:            "test-device",
		RegistrationKey: "test-master-key",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/devices/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleRegister(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var profile device.Profile
	if err := json.NewDecoder(w.Body).Decode(&profile); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if profile.Name != "test-device" {
		t.Errorf("expected name 'test-device', got %q", profile.Name)
	}
	if profile.Token == "" {
		t.Errorf("expected non-empty token")
	}
}

// TestHandleRegister_InvalidKey tests 403 on wrong registration key.
func TestHandleRegister_InvalidKey(t *testing.T) {
	srv := newTestServer(t)

	body, _ := json.Marshal(registerRequest{
		Name:            "test-device",
		RegistrationKey: "wrong-key",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/devices/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleRegister(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// TestHandleRegister_EmptyName tests 400 on empty device name.
func TestHandleRegister_EmptyName(t *testing.T) {
	srv := newTestServer(t)

	body, _ := json.Marshal(registerRequest{
		Name:            "",
		RegistrationKey: "test-master-key",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/devices/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleRegister(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestHandleRegister_WrongMethod tests 405 on GET.
func TestHandleRegister_WrongMethod(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/devices/register", nil)
	w := httptest.NewRecorder()
	srv.handleRegister(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// TestHandleRegister_BodyTooLarge tests 413 on oversized body.
func TestHandleRegister_BodyTooLarge(t *testing.T) {
	srv := newTestServer(t)

	// Create a valid JSON body larger than maxJSONBodySize (1 MB).
	bigBody := `{"name":"` + strings.Repeat("x", maxJSONBodySize) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/devices/register", strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleRegister(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleChat_WrongMethod tests 405 on GET.
func TestHandleChat_WrongMethod(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat", nil)
	req = withDevice(req)
	w := httptest.NewRecorder()
	srv.handleChat(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// TestHandleChat_MissingMessage tests 400 on empty message.
func TestHandleChat_MissingMessage(t *testing.T) {
	srv := newTestServer(t)

	body, _ := json.Marshal(chatRequest{Message: ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", bytes.NewReader(body))
	req = withDevice(req)
	w := httptest.NewRecorder()
	srv.handleChat(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestHandleChat_NoAuth tests 401 without device context.
func TestHandleChat_NoAuth(t *testing.T) {
	srv := newTestServer(t)

	body, _ := json.Marshal(chatRequest{Message: "hello"})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleChat(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestHandleChat_BodyTooLarge tests 413 on oversized body.
func TestHandleChat_BodyTooLarge(t *testing.T) {
	srv := newTestServer(t)

	bigBody := `{"message":"` + strings.Repeat("x", maxJSONBodySize) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(bigBody))
	req = withDevice(req)
	w := httptest.NewRecorder()
	srv.handleChat(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleDeviceMe_Get tests retrieving device profile.
func TestHandleDeviceMe_Get(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/devices/me", nil)
	req = withDevice(req)
	w := httptest.NewRecorder()
	srv.handleDeviceMe(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var profile device.Profile
	if err := json.NewDecoder(w.Body).Decode(&profile); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if profile.Name != "test-device" {
		t.Errorf("expected name 'test-device', got %q", profile.Name)
	}
}

// TestHandleDeviceMe_Put tests updating device profile fields.
func TestHandleDeviceMe_Put(t *testing.T) {
	srv := newTestServer(t)

	body, _ := json.Marshal(deviceUpdateRequest{
		Model:    "claude-sonnet-4-6",
		Location: "Baltimore, MD",
	})
	req := httptest.NewRequest(http.MethodPut, "/v1/devices/me", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withDevice(req)
	w := httptest.NewRecorder()
	srv.handleDeviceMe(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var profile device.Profile
	if err := json.NewDecoder(w.Body).Decode(&profile); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if profile.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model 'claude-sonnet-4-6', got %q", profile.Model)
	}
	if profile.Location != "Baltimore, MD" {
		t.Errorf("expected location 'Baltimore, MD', got %q", profile.Location)
	}
}

// TestHandleDeviceMe_WrongMethod tests 405 on DELETE.
func TestHandleDeviceMe_WrongMethod(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/v1/devices/me", nil)
	req = withDevice(req)
	w := httptest.NewRecorder()
	srv.handleDeviceMe(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// TestHandleSessions_Empty tests listing sessions with none saved.
func TestHandleSessions_Empty(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions", nil)
	req = withDevice(req)
	w := httptest.NewRecorder()
	srv.handleSessions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// TestHandleSession_NotFound tests 404 for nonexistent session.
func TestHandleSession_NotFound(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/nonexistent", nil)
	req = withDevice(req)
	w := httptest.NewRecorder()
	srv.handleSession(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// TestHandleSession_CreateAndGet tests creating then retrieving a session.
func TestHandleSession_CreateAndGet(t *testing.T) {
	srv := newTestServer(t)

	// Create a session by saving directly to the store.
	dev := &device.Profile{ID: "testdev"}
	store := srv.devices.SessionStore(dev.ID)
	sess := session.NewSession("abc12345", "qwen3:32b")
	sess.AddMessage("user", "hello world")
	if err := store.Save(sess); err != nil {
		t.Fatalf("save session: %v", err)
	}

	// Retrieve it via the handler.
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/abc1", nil)
	req = withDevice(req)
	w := httptest.NewRecorder()
	srv.handleSession(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result session.Session
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.ID != "abc12345" {
		t.Errorf("expected ID 'abc12345', got %q", result.ID)
	}
}

// TestHandleSession_Delete tests deleting a session.
func TestHandleSession_Delete(t *testing.T) {
	srv := newTestServer(t)

	dev := &device.Profile{ID: "testdev"}
	store := srv.devices.SessionStore(dev.ID)
	sess := session.NewSession("del12345", "qwen3:32b")
	if err := store.Save(sess); err != nil {
		t.Fatalf("save session: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/v1/sessions/del1", nil)
	req = withDevice(req)
	w := httptest.NewRecorder()
	srv.handleSession(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	// Verify it's gone.
	_, err := store.FindByPrefix("del1")
	if err == nil {
		t.Errorf("expected error loading deleted session")
	}
}

// TestHandleSpeak_WrongContentType tests 415 on wrong Content-Type.
func TestHandleSpeak_WrongContentType(t *testing.T) {
	srv := &Server{
		piper: audio.NewPiperTTS("/nonexistent/model", ""),
	}

	body, _ := json.Marshal(map[string]string{"text": "hello"})
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speak", bytes.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	req = withDevice(req)
	w := httptest.NewRecorder()
	srv.handleSpeak(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleSpeak_TextTooLong tests 400 on text exceeding maxTTSTextLen.
func TestHandleSpeak_TextTooLong(t *testing.T) {
	srv := &Server{piper: audio.NewPiperTTS("/nonexistent/model", "")}

	longText := strings.Repeat("a", maxTTSTextLen+1)
	body, _ := json.Marshal(map[string]string{"text": longText})
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speak", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withDevice(req)
	w := httptest.NewRecorder()
	srv.handleSpeak(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleTranscribe_WrongContentType tests 415 on wrong Content-Type.
func TestHandleTranscribe_WrongContentType(t *testing.T) {
	srv := &Server{
		whisper: dummyWhisperClient(t),
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcribe", bytes.NewReader([]byte("data")))
	req.Header.Set("Content-Type", "text/plain")
	req = withDevice(req)
	w := httptest.NewRecorder()
	srv.handleTranscribe(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415, got %d: %s", w.Code, w.Body.String())
	}
}

// TestParseChatRequest tests the parseChatRequest helper.
func TestParseChatRequest(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"valid", `{"message":"hello"}`, false},
		{"empty message", `{"message":""}`, true},
		{"whitespace message", `{"message":"   "}`, true},
		{"invalid json", `not json`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			result, err := parseChatRequest(req)
			if tt.wantErr && err == nil {
				t.Errorf("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && result.Message != "hello" {
				t.Errorf("expected message 'hello', got %q", result.Message)
			}
		})
	}
}

// TestResolveChatConfig tests config resolution from request and device profile.
func TestResolveChatConfig(t *testing.T) {
	// Request model overrides device model.
	req := &chatRequest{Model: "claude-sonnet-4-6"}
	dev := &device.Profile{Model: "qwen3:32b", Location: "Baltimore"}
	cfg := resolveChatConfig(req, dev)

	if cfg.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model 'claude-sonnet-4-6', got %q", cfg.Model)
	}
	if cfg.Location != "Baltimore" {
		t.Errorf("expected location 'Baltimore', got %q", cfg.Location)
	}
	if !cfg.EnableTools {
		t.Errorf("expected tools enabled by default")
	}

	// Device model used when request model empty.
	req2 := &chatRequest{}
	cfg2 := resolveChatConfig(req2, dev)
	if cfg2.Model != "qwen3:32b" {
		t.Errorf("expected device model 'qwen3:32b', got %q", cfg2.Model)
	}
}

func dummyWhisperClient(t *testing.T) *audio.WhisperClient {
	t.Helper()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"text": "mock"})
	}))
	t.Cleanup(mockServer.Close)
	return audio.NewWhisperClient(mockServer.URL)
}
