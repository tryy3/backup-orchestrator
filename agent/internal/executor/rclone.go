package executor

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteRcloneConfig writes the rclone configuration text to {dataDir}/rclone.conf.
func WriteRcloneConfig(dataDir, configText string) error {
	path := RcloneConfigPath(dataDir)
	if err := os.WriteFile(path, []byte(configText), 0600); err != nil {
		return fmt.Errorf("writing rclone config: %w", err)
	}
	return nil
}

// RcloneConfigPath returns the path to the rclone config file.
func RcloneConfigPath(dataDir string) string {
	return filepath.Join(dataDir, "rclone.conf")
}
