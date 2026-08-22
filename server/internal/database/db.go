package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/tryy3/backup-orchestrator/server/internal/crypto"
)

// DB wraps a sql.DB connection to provide domain-specific query methods.
type DB struct {
	*sql.DB
	encryptionKey []byte // 32-byte AES-256 key; nil disables encryption

	syncer      remoteSync
	syncCancel  context.CancelFunc
	syncMu      sync.Mutex
	lastSyncOK  time.Time
	lastSyncErr error
}

// Close stops the sync loop, best-effort pushes local changes, then closes the connection.
func (db *DB) Close() error {
	if db.syncCancel != nil {
		db.syncCancel()
		db.syncCancel = nil
	}
	if db.syncer != nil {
		_ = db.syncer.Push(context.Background())
		db.syncer = nil
	}
	if db.DB == nil {
		return nil
	}
	return db.DB.Close()
}

// encrypt encrypts a plaintext value if an encryption key is configured.
func (db *DB) encrypt(plaintext string) (string, error) {
	if len(db.encryptionKey) != 32 || plaintext == "" {
		return plaintext, nil
	}
	return crypto.Encrypt(db.encryptionKey, plaintext)
}

// decrypt decrypts a value if it carries the "enc:" prefix.
// Plaintext values are returned as-is for backward compatibility.
func (db *DB) decrypt(value string) (string, error) {
	if len(db.encryptionKey) != 32 || value == "" {
		return value, nil
	}
	return crypto.Decrypt(db.encryptionKey, value)
}

// decryptPtr decrypts a nullable string pointer.
func (db *DB) decryptPtr(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	decrypted, err := db.decrypt(*value)
	if err != nil {
		return nil, err
	}
	return &decrypted, nil
}

// encryptPtr encrypts a nullable string pointer.
func (db *DB) encryptPtr(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	encrypted, err := db.encrypt(*value)
	if err != nil {
		return nil, err
	}
	return &encrypted, nil
}

// migrateEncryption re-encrypts any plaintext values found in the database.
func (db *DB) migrateEncryption(ctx context.Context) error {
	// Migrate repository passwords.
	if err := db.migrateRepositoryPasswords(ctx); err != nil {
		return err
	}

	// Migrate agent rclone configs.
	return db.migrateAgentRcloneConfigs(ctx)
}

// migrateRepositoryPasswords encrypts any plaintext repository passwords.
func (db *DB) migrateRepositoryPasswords(ctx context.Context) error {
	rows, err := db.QueryContext(ctx, "SELECT id, password FROM repositories")
	if err != nil {
		return fmt.Errorf("query repository passwords: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type idVal struct {
		id, val string
	}
	var updates []idVal
	for rows.Next() {
		var id, password string
		if err = rows.Scan(&id, &password); err != nil {
			return fmt.Errorf("scan repository password: %w", err)
		}
		if password != "" && !crypto.IsEncrypted(password) {
			encrypted, encErr := crypto.Encrypt(db.encryptionKey, password)
			if encErr != nil {
				return fmt.Errorf("encrypt repository %s password: %w", id, encErr)
			}
			updates = append(updates, idVal{id, encrypted})
		}
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("iterate repository passwords: %w", err)
	}

	for _, u := range updates {
		if _, err = db.ExecContext(ctx, "UPDATE repositories SET password = ? WHERE id = ?", u.val, u.id); err != nil {
			return fmt.Errorf("update repository %s password: %w", u.id, err)
		}
	}
	if len(updates) > 0 {
		slog.Info("encrypted repository passwords", "count", len(updates))
	}
	return nil
}

// migrateAgentRcloneConfigs encrypts any plaintext agent rclone configs.
func (db *DB) migrateAgentRcloneConfigs(ctx context.Context) error {
	rows, err := db.QueryContext(ctx, "SELECT id, rclone_config FROM agents WHERE rclone_config IS NOT NULL AND rclone_config != ''")
	if err != nil {
		return fmt.Errorf("query agent rclone configs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type idVal struct {
		id, val string
	}
	var updates []idVal
	for rows.Next() {
		var id, config string
		if err = rows.Scan(&id, &config); err != nil {
			return fmt.Errorf("scan agent rclone config: %w", err)
		}
		if !crypto.IsEncrypted(config) {
			encrypted, encErr := crypto.Encrypt(db.encryptionKey, config)
			if encErr != nil {
				return fmt.Errorf("encrypt agent %s rclone config: %w", id, encErr)
			}
			updates = append(updates, idVal{id, encrypted})
		}
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("iterate agent rclone configs: %w", err)
	}

	for _, u := range updates {
		if _, err = db.ExecContext(ctx, "UPDATE agents SET rclone_config = ? WHERE id = ?", u.val, u.id); err != nil {
			return fmt.Errorf("update agent %s rclone config: %w", u.id, err)
		}
	}
	if len(updates) > 0 {
		slog.Info("encrypted agent rclone configs", "count", len(updates))
	}
	return nil
}
