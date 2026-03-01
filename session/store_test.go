package session

import (
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return NewStoreAt(dir)
}

func TestSaveAndLoad(t *testing.T) {
	store := newTestStore(t)

	sess := NewSession("test-id", "qwen3:32b")
	sess.AddMessage("user", "hello")
	sess.AddMessage("assistant", "hi there")

	if err := store.Save(sess); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load("test-id")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.ID != "test-id" {
		t.Errorf("expected ID 'test-id', got %q", loaded.ID)
	}
	if loaded.Model != "qwen3:32b" {
		t.Errorf("expected model 'qwen3:32b', got %q", loaded.Model)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Role != "user" || loaded.Messages[0].Content != "hello" {
		t.Errorf("message 0: expected user/hello, got %s/%s", loaded.Messages[0].Role, loaded.Messages[0].Content)
	}
}

func TestFindByPrefix_Exact(t *testing.T) {
	store := newTestStore(t)

	sess := NewSession("abc12345", "qwen3:32b")
	if err := store.Save(sess); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := store.FindByPrefix("abc1")
	if err != nil {
		t.Fatalf("find by prefix: %v", err)
	}
	if found.ID != "abc12345" {
		t.Errorf("expected ID 'abc12345', got %q", found.ID)
	}
}

func TestFindByPrefix_Ambiguous(t *testing.T) {
	store := newTestStore(t)

	sess1 := NewSession("abc12345", "qwen3:32b")
	sess2 := NewSession("abc67890", "qwen3:32b")
	store.Save(sess1)
	store.Save(sess2)

	_, err := store.FindByPrefix("abc")
	if err == nil {
		t.Fatalf("expected error for ambiguous prefix")
	}
}

func TestFindByPrefix_NoMatch(t *testing.T) {
	store := newTestStore(t)

	sess := NewSession("abc12345", "qwen3:32b")
	store.Save(sess)

	_, err := store.FindByPrefix("xyz")
	if err == nil {
		t.Fatalf("expected error for no match")
	}
}

func TestList_SortedByUpdatedAt(t *testing.T) {
	store := newTestStore(t)

	sess1 := NewSession("older", "qwen3:32b")
	sess1.UpdatedAt = time.Now().Add(-1 * time.Hour)
	store.Save(sess1)

	sess2 := NewSession("newer", "qwen3:32b")
	sess2.UpdatedAt = time.Now()
	store.Save(sess2)

	sessions, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].ID != "newer" {
		t.Errorf("expected newest first, got %q", sessions[0].ID)
	}
}

func TestList_NonexistentDir(t *testing.T) {
	store := NewStoreAt(t.TempDir() + "/nonexistent")

	sessions, err := store.List()
	if err != nil {
		t.Fatalf("list on nonexistent dir should not error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestDelete(t *testing.T) {
	store := newTestStore(t)

	sess := NewSession("to-delete", "qwen3:32b")
	store.Save(sess)

	if err := store.Delete("to-delete"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := store.Load("to-delete")
	if err == nil {
		t.Errorf("expected error loading deleted session")
	}
}
