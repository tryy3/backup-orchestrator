package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	original := &Identity{AgentID: "agent-123"}
	original.SetAPIKey("key-456")

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
	if loaded.GetAPIKey() != original.GetAPIKey() {
		t.Errorf("APIKey: got %q, want %q", loaded.GetAPIKey(), original.GetAPIKey())
	}
}

func TestSaveFilePermissions(t *testing.T) {
	dir := t.TempDir()
	id := &Identity{AgentID: "a"}
	id.SetAPIKey("b")
	if err := Save(dir, id); err != nil {
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

	old := &Identity{AgentID: "old"}
	old.SetAPIKey("old-key")
	if err := Save(dir, old); err != nil {
		t.Fatalf("Save: %v", err)
	}
	newID := &Identity{AgentID: "new"}
	newID.SetAPIKey("new-key")
	if err := Save(dir, newID); err != nil {
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

// TestAPIKeyConcurrentAccess verifies that concurrent SetAPIKey / GetAPIKey
// calls do not race. Run with -race to exercise the detector.
func TestAPIKeyConcurrentAccess(t *testing.T) {
	id := &Identity{AgentID: "agent-123"}
	id.SetAPIKey("initial-key")

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := range goroutines {
		go func(n int) {
			defer wg.Done()
			id.SetAPIKey(fmt.Sprintf("key-%d", n))
		}(i)
		go func() {
			defer wg.Done()
			_ = id.GetAPIKey()
		}()
	}

	wg.Wait()
}
