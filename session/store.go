package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Store handles session persistence to disk.
type Store struct {
	dir string
}

// NewStore creates a store using XDG_DATA_HOME or ~/.local/share.
func NewStore() *Store {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	dir := filepath.Join(base, "localfreshllm", "history")
	return &Store{dir: dir}
}

// Save writes a session to disk atomically.
func (s *Store) Save(sess *Session) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return fmt.Errorf("create history dir: %w", err)
	}

	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	target := filepath.Join(s.dir, sess.ID+".json")
	tmp := target + ".tmp"

	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

// Load reads a session by exact ID.
func (s *Store) Load(id string) (*Session, error) {
	path := filepath.Join(s.dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session: %w", err)
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}

	return &sess, nil
}

// FindByPrefix finds a session whose ID starts with the given prefix.
func (s *Store) FindByPrefix(prefix string) (*Session, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("read history dir: %w", err)
	}

	var matches []string
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".json")
		if strings.HasPrefix(name, prefix) {
			matches = append(matches, name)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no session matching prefix %q", prefix)
	case 1:
		return s.Load(matches[0])
	default:
		return nil, fmt.Errorf("ambiguous prefix %q: matches %d sessions", prefix, len(matches))
	}
}

// List returns all sessions sorted by most recent first.
func (s *Store) List() ([]*Session, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read history dir: %w", err)
	}

	var sessions []*Session
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		sess, err := s.Load(id)
		if err != nil {
			continue
		}
		sessions = append(sessions, sess)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}
