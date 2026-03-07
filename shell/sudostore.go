package shell

import (
	"sync"
	"time"
)

const defaultSudoTTL = 15 * time.Minute

// SudoStore holds cached sudo passwords per device, with expiry.
// Passwords are kept in memory only — never serialized or logged.
type SudoStore struct {
	mu    sync.RWMutex
	cache map[string]*sudoEntry
}

type sudoEntry struct {
	password  string
	expiresAt time.Time
}

// NewSudoStore creates a new empty store.
func NewSudoStore() *SudoStore {
	return &SudoStore{cache: make(map[string]*sudoEntry)}
}

// Set stores a password for the given device ID with the default TTL.
func (s *SudoStore) Set(deviceID, password string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[deviceID] = &sudoEntry{
		password:  password,
		expiresAt: time.Now().Add(defaultSudoTTL),
	}
}

// Get returns the cached password, or "" if expired or not set.
func (s *SudoStore) Get(deviceID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.cache[deviceID]
	if !ok || time.Now().After(e.expiresAt) {
		return ""
	}
	return e.password
}

// Clear removes the cached password for a device.
func (s *SudoStore) Clear(deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, deviceID)
}

// ValidatePassword tests whether the password works for sudo.
// Returns nil on success.
func ValidatePassword(password string) error {
	return validateSudo(password)
}
