package localconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tryy3/backup-orchestrator/agent/internal/atomicfile"
	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// Save marshals the AgentConfig to JSON and writes it atomically to {dataDir}/config.json.
func Save(dataDir string, cfg *backupv1.AgentConfig) error {
	path := filepath.Join(dataDir, "config.json")
	data, err := protojson.MarshalOptions{
		Indent: "  ",
	}.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := atomicfile.Write(path, data, 0o600); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}
	return nil
}

// Load reads and unmarshals the AgentConfig from {dataDir}/config.json.
// Returns an error if the file doesn't exist.
func Load(dataDir string) (*backupv1.AgentConfig, error) {
	path := filepath.Join(dataDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file not found: %w", err)
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg backupv1.AgentConfig
	if err := protojson.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}
	return &cfg, nil
}
