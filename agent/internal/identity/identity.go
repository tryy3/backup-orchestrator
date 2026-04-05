package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Identity holds the agent's enrollment credentials.
type Identity struct {
	AgentID string `json:"agent_id"`
	APIKey  string `json:"api_key"`
}

// Load reads the identity from {dataDir}/identity.json.
// Returns nil, nil if the file doesn't exist (first run).
func Load(dataDir string) (*Identity, error) {
	path := filepath.Join(dataDir, "identity.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading identity file: %w", err)
	}

	var id Identity
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, fmt.Errorf("parsing identity file: %w", err)
	}
	return &id, nil
}

// Save writes the identity to {dataDir}/identity.json.
func Save(dataDir string, id *Identity) error {
	path := filepath.Join(dataDir, "identity.json")
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling identity: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing identity file: %w", err)
	}
	return nil
}
