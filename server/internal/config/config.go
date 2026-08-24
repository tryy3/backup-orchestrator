package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// defaultAllowedOrigins are the origins permitted when BACKUP_ALLOWED_ORIGINS is not set.
// These cover the standard Vite dev-server ports so local development works out of the box.
var defaultAllowedOrigins = []string{
	"http://localhost:5173",
	"http://localhost:3000",
}

// Config holds server configuration values.
type Config struct {
	Driver         Driver
	DBPath         string
	DBURL          string
	DBAuthToken    string
	SyncInterval   time.Duration
	HTTPPort       string
	GRPCPort       string
	AllowedOrigins []string // CORS allowed origins
	EncryptionKey  []byte   // 32-byte AES-256 key for encrypting secrets at rest
}

type Driver string

const (
	DriverSQLite    Driver = "sqlite"
	DriverTurso     Driver = "turso"
	DriverTursoSync Driver = "turso-sync"
)

func ParseDriver(raw string) (Driver, error) {
	switch strings.TrimSpace(raw) {
	case "", string(DriverSQLite):
		return DriverSQLite, nil
	case string(DriverTurso):
		return DriverTurso, nil
	case string(DriverTursoSync):
		return DriverTursoSync, nil
	default:
		return "", fmt.Errorf("BACKUP_DB_DRIVER must be sqlite, turso, or turso-sync, got %q", raw)
	}
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	driver, err := ParseDriver(os.Getenv("BACKUP_DB_DRIVER"))
	if err != nil {
		return nil, err
	}
	dbPath := getenv("BACKUP_DB_PATH", "/var/lib/backup-orchestrator/server.db")
	if driver == DriverTursoSync {
		dbPath = strings.TrimSpace(os.Getenv("BACKUP_DB_PATH"))
	}
	url := strings.TrimSpace(os.Getenv("BACKUP_DB_URL"))
	token := strings.TrimSpace(os.Getenv("BACKUP_DB_AUTH_TOKEN"))
	interval := 30 * time.Second
	if raw := os.Getenv("BACKUP_DB_SYNC_INTERVAL"); raw != "" {
		interval, err = time.ParseDuration(raw)
		if err != nil || interval <= 0 {
			return nil, fmt.Errorf("BACKUP_DB_SYNC_INTERVAL must be a positive duration: %w", err)
		}
	}
	switch driver {
	case DriverTurso:
		if url == "" || token == "" {
			return nil, fmt.Errorf("BACKUP_DB_URL and BACKUP_DB_AUTH_TOKEN are required when BACKUP_DB_DRIVER=turso")
		}
	case DriverTursoSync:
		if dbPath == "" || url == "" || token == "" {
			return nil, fmt.Errorf("BACKUP_DB_PATH, BACKUP_DB_URL, and BACKUP_DB_AUTH_TOKEN are required when BACKUP_DB_DRIVER=turso-sync")
		}
	}
	key, err := loadEncryptionKey(driver, dbPath)
	if err != nil {
		return nil, fmt.Errorf("load encryption key: %w", err)
	}
	return &Config{
		Driver:         driver,
		DBPath:         dbPath,
		DBURL:          url,
		DBAuthToken:    token,
		SyncInterval:   interval,
		HTTPPort:       getenv("BACKUP_HTTP_PORT", "8080"),
		GRPCPort:       getenv("BACKUP_GRPC_PORT", "8443"),
		AllowedOrigins: getAllowedOrigins(),
		EncryptionKey:  key,
	}, nil
}

// loadEncryptionKey resolves the 32-byte AES-256 encryption key in order:
//  1. BACKUP_ENCRYPTION_KEY env var (64 hex characters)
//  2. Key file alongside the database (<db_dir>/encryption.key)
//  3. Auto-generate a random key and persist it to the key file
func loadEncryptionKey(driver Driver, dbPath string) ([]byte, error) {
	// 1. Environment variable.
	if hexKey := os.Getenv("BACKUP_ENCRYPTION_KEY"); hexKey != "" {
		key, err := hex.DecodeString(hexKey)
		if err != nil {
			return nil, fmt.Errorf("BACKUP_ENCRYPTION_KEY is not valid hex: %w", err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("BACKUP_ENCRYPTION_KEY must be 64 hex characters (32 bytes), got %d bytes", len(key))
		}
		return key, nil
	}
	if driver == DriverTurso {
		return nil, fmt.Errorf("BACKUP_ENCRYPTION_KEY is required when BACKUP_DB_DRIVER=turso")
	}

	// 2. Key file next to the database.
	keyPath := filepath.Join(filepath.Dir(dbPath), "encryption.key")
	data, err := os.ReadFile(keyPath)
	if err == nil {
		key, decErr := hex.DecodeString(strings.TrimSpace(string(data)))
		if decErr == nil && len(key) == 32 {
			return key, nil
		}
	}

	// 3. Generate and persist.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate encryption key: %w", err)
	}
	hexKey := hex.EncodeToString(key) + "\n"
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return nil, fmt.Errorf("create key directory: %w", err)
	}
	if err := os.WriteFile(keyPath, []byte(hexKey), 0o600); err != nil {
		return nil, fmt.Errorf("write encryption key file %s: %w", keyPath, err)
	}
	slog.Info("generated new encryption key", "path", keyPath)
	return key, nil
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
