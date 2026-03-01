package device

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/rabidclock/localfreshllm/session"
)

// Profile represents a registered device.
type Profile struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Model      string    `json:"model,omitempty"`
	Location   string    `json:"location,omitempty"`
	Persona    string    `json:"persona,omitempty"`
	Token      string    `json:"token"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// Store manages device profiles on disk.
type Store struct {
	baseDir string
	mu      sync.RWMutex
}

// NewStore creates a device store at the default XDG data directory.
func NewStore() *Store {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return &Store{baseDir: filepath.Join(base, "localfreshllm", "devices")}
}

// Register creates a new device profile after validating the registration key.
func (s *Store) Register(name, registrationKey, masterKey string) (*Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if registrationKey != masterKey {
		return nil, fmt.Errorf("invalid registration key")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("device name is required")
	}

	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	now := time.Now()
	p := &Profile{
		ID:         uuid.New().String()[:8],
		Name:       name,
		Token:      token,
		CreatedAt:  now,
		LastSeenAt: now,
	}

	if err := s.save(p); err != nil {
		return nil, err
	}

	return p, nil
}

// GetByToken finds a device by its bearer token.
func (s *Store) GetByToken(token string) (*Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	profiles, err := s.listAll()
	if err != nil {
		return nil, err
	}

	for _, p := range profiles {
		if p.Token == token {
			return p, nil
		}
	}

	return nil, fmt.Errorf("device not found")
}

// Get retrieves a device by ID.
func (s *Store) Get(id string) (*Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.load(id)
}

// Update persists changes to a device profile.
func (s *Store) Update(p *Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.save(p)
}

// List returns all registered device profiles.
func (s *Store) List() ([]*Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.listAll()
}

// Delete removes a device and its session history.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Join(s.baseDir, id)
	return os.RemoveAll(dir)
}

// SessionStore returns a session store scoped to the given device.
func (s *Store) SessionStore(deviceID string) *session.Store {
	dir := filepath.Join(s.baseDir, deviceID, "history")
	return session.NewStoreAt(dir)
}

func (s *Store) save(p *Profile) error {
	dir := filepath.Join(s.baseDir, p.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create device dir: %w", err)
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal device: %w", err)
	}

	target := filepath.Join(dir, "device.json")
	tmp := target + ".tmp"

	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write device: %w", err)
	}

	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

func (s *Store) load(id string) (*Profile, error) {
	path := filepath.Join(s.baseDir, id, "device.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read device: %w", err)
	}

	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("unmarshal device: %w", err)
	}

	return &p, nil
}

func (s *Store) listAll() ([]*Profile, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read devices dir: %w", err)
	}

	var profiles []*Profile
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := s.load(e.Name())
		if err != nil {
			continue
		}
		profiles = append(profiles, p)
	}

	return profiles, nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
