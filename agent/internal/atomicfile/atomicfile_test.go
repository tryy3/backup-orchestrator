package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	if err := Write(path, []byte(`{"hello":"world"}`), 0o600); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != `{"hello":"world"}` {
		t.Errorf("content: got %q, want %q", got, `{"hello":"world"}`)
	}
}

func TestWriteFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.json")

	if err := Write(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("Write: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions: got %o, want 0600", perm)
	}
}

func TestWriteOverwritePreservesContentOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	if err := Write(path, []byte("original"), 0o600); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if err := Write(path, []byte("updated"), 0o600); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "updated" {
		t.Errorf("content: got %q, want %q", got, "updated")
	}
}

// TestWriteNoTempFilesLeft asserts that no *.tmp.* files are left behind after
// a successful write.
func TestWriteNoTempFilesLeft(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	if err := Write(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("Write: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "data.json" {
			t.Errorf("unexpected leftover file: %s", e.Name())
		}
	}
}

// TestWriteOriginalPreservedWhenDirReadOnly checks that a failed write (here
// simulated by making the directory read-only so CreateTemp fails) does not
// corrupt or remove an existing file.
func TestWriteOriginalPreservedWhenCreateTempFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	original := []byte("original content")
	if err := Write(path, original, 0o600); err != nil {
		t.Fatalf("first Write: %v", err)
	}

	// Make directory read-only so CreateTemp fails.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := Write(path, []byte("new content"), 0o600); err == nil {
		t.Fatal("expected error writing to read-only dir")
	}

	// Restore permissions and verify original content is intact.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("Chmod restore: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(original) {
		t.Errorf("content after failed write: got %q, want %q", got, original)
	}
}
