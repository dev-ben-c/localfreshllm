package server

import (
	"fmt"
	"log"
	"net/http"

	"github.com/rabidclock/localfreshllm/audio"
	"github.com/rabidclock/localfreshllm/device"
	"github.com/rabidclock/localfreshllm/service"
)

// Server is the LocalFreshLLM HTTP API server.
type Server struct {
	addr        string
	masterKey   string
	chatService *service.ChatService
	devices     *device.Store
	whisper     *audio.WhisperClient
	piper       *audio.PiperTTS
}

// AudioConfig holds optional audio service configuration.
type AudioConfig struct {
	WhisperURL   string
	PiperModel   string
	PiperSpeaker string
}

// New creates a new server instance.
func New(addr, masterKey string) *Server {
	return &Server{
		addr:        addr,
		masterKey:   masterKey,
		chatService: service.New(),
		devices:     device.NewStore(),
	}
}

// NewWithAudio creates a server with optional audio services configured.
func NewWithAudio(addr, masterKey string, audioCfg AudioConfig) *Server {
	s := New(addr, masterKey)
	if audioCfg.WhisperURL != "" {
		s.whisper = audio.NewWhisperClient(audioCfg.WhisperURL)
	}
	if audioCfg.PiperModel != "" {
		s.piper = audio.NewPiperTTS(audioCfg.PiperModel, audioCfg.PiperSpeaker)
	}
	return s
}

// Run starts the HTTP server and blocks until it exits.
func (s *Server) Run() error {
	mux := http.NewServeMux()

	// Public endpoints.
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/chat", s.handleChatPage)
	mux.HandleFunc("/v1/devices/register", s.handleRegister)

	// Authenticated endpoints — wrap with auth middleware.
	authMux := http.NewServeMux()
	authMux.HandleFunc("/v1/chat", s.handleChat)
	authMux.HandleFunc("/v1/models", s.handleModels)
	authMux.HandleFunc("/v1/devices/me", s.handleDeviceMe)
	authMux.HandleFunc("/v1/sessions", s.handleSessions)
	authMux.HandleFunc("/v1/sessions/", s.handleSession)
	authMux.HandleFunc("/v1/audio/transcribe", s.handleTranscribe)
	authMux.HandleFunc("/v1/audio/speak", s.handleSpeak)

	mux.Handle("/v1/chat", s.authMiddleware(authMux))
	mux.Handle("/v1/models", s.authMiddleware(authMux))
	mux.Handle("/v1/devices/me", s.authMiddleware(authMux))
	mux.Handle("/v1/sessions", s.authMiddleware(authMux))
	mux.Handle("/v1/sessions/", s.authMiddleware(authMux))
	mux.Handle("/v1/audio/transcribe", s.authMiddleware(authMux))
	mux.Handle("/v1/audio/speak", s.authMiddleware(authMux))

	log.Printf("LocalFreshLLM server listening on %s", s.addr)

	devs, _ := s.devices.List()
	log.Printf("Registered devices: %d", len(devs))

	if s.whisper != nil {
		log.Printf("Speech-to-text: %s", s.whisper.BaseURL)
	}
	if s.piper != nil {
		log.Printf("Text-to-speech: piper model %s", s.piper.ModelPath)
	}

	return fmt.Errorf("server exited: %w", http.ListenAndServe(s.addr, mux))
}
