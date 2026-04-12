package config

import (
	"os"
	"strings"
)

// defaultAllowedOrigins are the origins permitted when BACKUP_ALLOWED_ORIGINS is not set.
// These cover the standard Vite dev-server ports so local development works out of the box.
var defaultAllowedOrigins = []string{
	"http://localhost:5173",
	"http://localhost:3000",
}

// Config holds server configuration values.
type Config struct {
	DBPath         string
	HTTPPort       string
	GRPCPort       string
	AllowedOrigins []string // CORS allowed origins
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		DBPath:         getenv("BACKUP_DB_PATH", "/var/lib/backup-orchestrator/server.db"),
		HTTPPort:       getenv("BACKUP_HTTP_PORT", "8080"),
		GRPCPort:       getenv("BACKUP_GRPC_PORT", "8443"),
		AllowedOrigins: getAllowedOrigins(),
	}
}

// getAllowedOrigins returns the list of CORS-allowed origins. When
// BACKUP_ALLOWED_ORIGINS is set it is parsed as a comma-separated list of
// origins; otherwise the dev-friendly defaults are used.
func getAllowedOrigins() []string {
	raw := os.Getenv("BACKUP_ALLOWED_ORIGINS")
	if raw == "" {
		return defaultAllowedOrigins
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		if o := strings.TrimSpace(p); o != "" {
			origins = append(origins, o)
		}
	}
	if len(origins) == 0 {
		return defaultAllowedOrigins
	}
	return origins
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
