package device

import (
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataDir)
	return NewStore()
}

func TestRegister_Valid(t *testing.T) {
	store := newTestStore(t)

	profile, err := store.Register("test-device", "master-key", "master-key")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if profile.Name != "test-device" {
		t.Errorf("expected name 'test-device', got %q", profile.Name)
	}
	if profile.Token == "" {
		t.Errorf("expected non-empty token")
	}
	if len(profile.ID) == 0 {
		t.Errorf("expected non-empty ID")
	}
}

func TestRegister_InvalidKey(t *testing.T) {
	store := newTestStore(t)

	_, err := store.Register("test-device", "wrong-key", "master-key")
	if err == nil {
		t.Fatalf("expected error for invalid key")
	}
}

func TestRegister_EmptyName(t *testing.T) {
	store := newTestStore(t)

	_, err := store.Register("", "master-key", "master-key")
	if err == nil {
		t.Fatalf("expected error for empty name")
	}

	_, err = store.Register("   ", "master-key", "master-key")
	if err == nil {
		t.Fatalf("expected error for whitespace-only name")
	}
}

func TestGetByToken(t *testing.T) {
	store := newTestStore(t)

	profile, err := store.Register("test-device", "key", "key")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	found, err := store.GetByToken(profile.Token)
	if err != nil {
		t.Fatalf("get by token: %v", err)
	}
	if found.ID != profile.ID {
		t.Errorf("expected ID %q, got %q", profile.ID, found.ID)
	}
}

func TestGetByToken_NotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.GetByToken("nonexistent-token")
	if err == nil {
		t.Fatalf("expected error for nonexistent token")
	}
}

func TestUpdate(t *testing.T) {
	store := newTestStore(t)

	profile, err := store.Register("test-device", "key", "key")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	profile.Model = "claude-sonnet-4-6"
	profile.Location = "Baltimore"
	if err := store.Update(profile); err != nil {
		t.Fatalf("update: %v", err)
	}

	loaded, err := store.Get(profile.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if loaded.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model 'claude-sonnet-4-6', got %q", loaded.Model)
	}
	if loaded.Location != "Baltimore" {
		t.Errorf("expected location 'Baltimore', got %q", loaded.Location)
	}
}

func TestDelete(t *testing.T) {
	store := newTestStore(t)

	profile, err := store.Register("test-device", "key", "key")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := store.Delete(profile.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = store.Get(profile.ID)
	if err == nil {
		t.Errorf("expected error after delete")
	}
}

func TestList(t *testing.T) {
	store := newTestStore(t)

	// Empty list.
	profiles, err := store.List()
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(profiles))
	}

	// Register two devices.
	_, err = store.Register("device-1", "key", "key")
	if err != nil {
		t.Fatalf("register 1: %v", err)
	}
	_, err = store.Register("device-2", "key", "key")
	if err != nil {
		t.Fatalf("register 2: %v", err)
	}

	profiles, err = store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(profiles))
	}
}

func TestSessionStore(t *testing.T) {
	store := newTestStore(t)

	profile, err := store.Register("test-device", "key", "key")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	sessStore := store.SessionStore(profile.ID)
	if sessStore == nil {
		t.Fatalf("expected non-nil session store")
	}
}
