package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

func openSQLite(opts Options) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", opts.Path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Connection pool settings for SQLite with WAL mode.
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	// Enable WAL mode for better concurrent read performance.
	if _, err := sqlDB.ExecContext(context.Background(), "PRAGMA journal_mode=WAL"); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	// Retry on SQLITE_BUSY for up to 5 seconds instead of failing immediately.
	if _, err := sqlDB.ExecContext(context.Background(), "PRAGMA busy_timeout=5000"); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	// Enable foreign key enforcement.
	if _, err := sqlDB.ExecContext(context.Background(), "PRAGMA foreign_keys=ON"); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	db := &DB{DB: sqlDB, encryptionKey: opts.EncryptionKey}

	if err := db.migrate(context.Background()); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	// Encrypt existing plaintext values if an encryption key is configured.
	if len(opts.EncryptionKey) == 32 {
		if err := db.migrateEncryption(context.Background()); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("encryption migration: %w", err)
		}
	}

	return db, nil
}
