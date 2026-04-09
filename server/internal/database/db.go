package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a sql.DB connection to provide domain-specific query methods.
type DB struct {
	*sql.DB
}

// New opens a SQLite database at the given path, enables WAL mode and
// foreign keys, configures connection pool, and runs migrations.
func New(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
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

	db := &DB{sqlDB}

	if err := db.migrate(context.Background()); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}
