package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	id, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != nil {
		t.Fatal("expected nil identity for missing file")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	original := &Identity{AgentID: "agent-123", APIKey: "key-456"}

	if err := Save(dir, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.AgentID != original.AgentID {
		t.Errorf("AgentID: got %q, want %q", loaded.AgentID, original.AgentID)
	}
	if loaded.APIKey != original.APIKey {
		t.Errorf("APIKey: got %q, want %q", loaded.APIKey, original.APIKey)
	}
}

func TestSaveFilePermissions(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, &Identity{AgentID: "a", APIKey: "b"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "identity.json"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("file permissions: got %o, want 0600", perm)
	}
}

func TestLoadCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.json")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	id, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for corrupted file")
	}
	if id != nil {
		t.Fatal("expected nil identity on error")
	}
}

func TestSaveOverwrite(t *testing.T) {
	dir := t.TempDir()

	if err := Save(dir, &Identity{AgentID: "old", APIKey: "old-key"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Save(dir, &Identity{AgentID: "new", APIKey: "new-key"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.AgentID != "new" {
		t.Errorf("AgentID: got %q, want %q", loaded.AgentID, "new")
	}
}
