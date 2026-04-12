package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds server configuration values.
type Config struct {
	DBPath        string
	HTTPPort      string
	GRPCPort      string
	EncryptionKey []byte // 32-byte AES-256 key for encrypting secrets at rest
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	dbPath := getenv("BACKUP_DB_PATH", "/var/lib/backup-orchestrator/server.db")
	key, err := loadEncryptionKey(dbPath)
	if err != nil {
		return nil, fmt.Errorf("load encryption key: %w", err)
	}
	return &Config{
		DBPath:        dbPath,
		HTTPPort:      getenv("BACKUP_HTTP_PORT", "8080"),
		GRPCPort:      getenv("BACKUP_GRPC_PORT", "8443"),
		EncryptionKey: key,
	}, nil
}

// loadEncryptionKey resolves the 32-byte AES-256 encryption key in order:
//  1. BACKUP_ENCRYPTION_KEY env var (64 hex characters)
//  2. Key file alongside the database (<db_dir>/encryption.key)
//  3. Auto-generate a random key and persist it to the key file
func loadEncryptionKey(dbPath string) ([]byte, error) {
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
	return key, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
