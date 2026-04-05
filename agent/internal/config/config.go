package config

import (
	"fmt"
	"os"
)

// Config holds the agent bootstrap configuration loaded from environment variables.
type Config struct {
	ServerURL string // BACKUP_SERVER_URL (required)
	AgentName string // BACKUP_AGENT_NAME (default: hostname)
	DataDir   string // BACKUP_DATA_DIR (default: /var/lib/backup-orchestrator)
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	serverURL := os.Getenv("BACKUP_SERVER_URL")
	if serverURL == "" {
		return nil, fmt.Errorf("BACKUP_SERVER_URL is required")
	}

	agentName := os.Getenv("BACKUP_AGENT_NAME")
	if agentName == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("getting hostname: %w", err)
		}
		agentName = hostname
	}

	dataDir := os.Getenv("BACKUP_DATA_DIR")
	if dataDir == "" {
		dataDir = "/var/lib/backup-orchestrator"
	}

	return &Config{
		ServerURL: serverURL,
		AgentName: agentName,
		DataDir:   dataDir,
	}, nil
}
