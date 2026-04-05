package config

import "os"

// Config holds server configuration values.
type Config struct {
	DBPath   string
	HTTPPort string
	GRPCPort string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		DBPath:   getenv("BACKUP_DB_PATH", "/var/lib/backup-orchestrator/server.db"),
		HTTPPort: getenv("BACKUP_HTTP_PORT", "8080"),
		GRPCPort: getenv("BACKUP_GRPC_PORT", "8443"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
