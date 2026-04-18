package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/tryy3/backup-orchestrator/agent/internal/atomicfile"
)

// Identity holds the agent's enrollment credentials.
// AgentID is set once at registration and never mutated; it is safe to read
// directly. APIKey may be written after approval and must be accessed via
// SetAPIKey / GetAPIKey to avoid data races.
type Identity struct {
	AgentID string `json:"agent_id"`
	apiKey  string
	mu      sync.RWMutex
}

// SetAPIKey stores the API key in a thread-safe manner.
func (id *Identity) SetAPIKey(key string) {
	id.mu.Lock()
	defer id.mu.Unlock()
	id.apiKey = key
}

// GetAPIKey returns the API key in a thread-safe manner.
func (id *Identity) GetAPIKey() string {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return id.apiKey
}

// identityJSON is the on-disk representation used for marshaling.
type identityJSON struct {
	AgentID string `json:"agent_id"`
	APIKey  string `json:"api_key"`
}

// MarshalJSON implements json.Marshaler for thread-safe serialization.
func (id *Identity) MarshalJSON() ([]byte, error) {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return json.Marshal(&identityJSON{AgentID: id.AgentID, APIKey: id.apiKey})
}

// UnmarshalJSON implements json.Unmarshaler.
func (id *Identity) UnmarshalJSON(data []byte) error {
	var tmp identityJSON
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	id.AgentID = tmp.AgentID
	id.mu.Lock()
	id.apiKey = tmp.APIKey
	id.mu.Unlock()
	return nil
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

// Save writes the identity to {dataDir}/identity.json atomically.
func Save(dataDir string, id *Identity) error {
	path := filepath.Join(dataDir, "identity.json")
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling identity: %w", err)
	}
	if err := atomicfile.Write(path, data, 0o600); err != nil {
		return fmt.Errorf("writing identity file: %w", err)
	}
	return nil
}
