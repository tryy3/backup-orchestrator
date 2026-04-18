// Package atomicfile provides crash-safe file writing via the temp-file +
// fsync + rename pattern. On POSIX systems os.Rename is atomic, so the
// destination file always contains either the previous valid content or the
// new valid content — a partial write is never visible to readers.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write atomically replaces the file at path with data using the
// write-to-temp + fsync + rename idiom. The temp file is created in the same
// directory as path so that the final rename stays on the same filesystem.
// perm is applied to the temp file before the rename.
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmp := f.Name()

	// Ensure cleanup on any error path.
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		return fmt.Errorf("setting temp file permissions: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}
	ok = true
	return nil
}
