package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds the agent bootstrap configuration loaded from environment variables.
type Config struct {
	ServerURL string // BACKUP_SERVER_URL (required)
	AgentName string // BACKUP_AGENT_NAME (default: hostname)
	DataDir   string // BACKUP_DATA_DIR (default: /var/lib/backup-orchestrator)

	// Outbox tunables. All fields have sensible defaults; override via env
	// vars only when running on resource-constrained or high-throughput
	// hosts.
	Outbox OutboxConfig
}

// OutboxConfig configures the in-memory + SQLite outbox. Only MemoryMax is
// settable here — the in-memory channel capacity is a bootstrap value because
// Go channels cannot be resized at runtime. All other outbox tunables are
// pushed from the server via AgentConfig (settings table + per-agent
// overrides) and hot-reloaded by the agent at runtime.
type OutboxConfig struct {
	MemoryMax int // OUTBOX_MEMORY_MAX (default 2000)
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

	outbox, err := loadOutbox()
	if err != nil {
		return nil, err
	}

	return &Config{
		ServerURL: serverURL,
		AgentName: agentName,
		DataDir:   dataDir,
		Outbox:    outbox,
	}, nil
}

func loadOutbox() (OutboxConfig, error) {
	cfg := OutboxConfig{
		MemoryMax: 2000,
	}

	if err := envInt("OUTBOX_MEMORY_MAX", &cfg.MemoryMax); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func envInt(name string, dst *int) error {
	v := os.Getenv(name)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if n <= 0 {
		return fmt.Errorf("%s: must be positive, got %d", name, n)
	}
	*dst = n
	return nil
}
