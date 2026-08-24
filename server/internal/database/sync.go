package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type syncFactory func(opts Options) (*sql.DB, remoteSync, error)

// newTursoSync is set by the tursogo backend in production; tests replace it with a fake.
var newTursoSync syncFactory

type remoteSync interface {
	Pull(ctx context.Context) error
	Push(ctx context.Context) error
}

func evaluateSyncStartup(localReady bool, pullErr error) error {
	if pullErr == nil {
		return nil
	}
	if localReady {
		return nil
	}
	return fmt.Errorf("turso-sync: remote unreachable and local database does not contain the expected schema: %w", pullErr)
}

func isCDCHistoryError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "without CDC history")
}

func localDBReady(ctx context.Context, db *sql.DB) bool {
	rows, err := db.QueryContext(ctx, "SELECT 1 FROM agents LIMIT 0")
	if err != nil {
		return false
	}
	return rows.Close() == nil
}

func openTursoSync(opts Options) (*DB, error) {
	if newTursoSync == nil {
		return nil, fmt.Errorf("turso-sync factory is not registered")
	}

	sqlDB, syncer, err := newTursoSync(opts)
	if err != nil {
		return nil, fmt.Errorf("open turso-sync database: %w", err)
	}

	ctx := context.Background()
	if err := enableForeignKeys(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	ready := localDBReady(ctx, sqlDB)
	pullErr := syncer.Pull(ctx)
	if err := evaluateSyncStartup(ready, pullErr); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if pullErr != nil && ready {
		if isCDCHistoryError(pullErr) {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("turso-sync cannot adopt a plain SQLite file (missing CDC history); run: just migrate-turso-sync <old.db> <new-empty.db>: %w", pullErr)
		}
		slog.Warn("turso sync pull failed at startup; continuing with local database", "error", pullErr)
	}

	db := &DB{DB: sqlDB, encryptionKey: opts.EncryptionKey, syncer: syncer}

	if err := db.migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	if len(opts.EncryptionKey) == 32 {
		if err := db.migrateEncryption(ctx); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("encryption migration: %w", err)
		}
	}

	if pullErr == nil {
		if pushErr := syncer.Push(ctx); pushErr != nil {
			db.setSyncStatus(pushErr)
		} else {
			db.setSyncStatus(nil)
		}
	}

	interval := opts.SyncInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	db.startSyncLoop(interval)

	return db, nil
}

func (db *DB) startSyncLoop(interval time.Duration) {
	if db.syncer == nil || interval <= 0 {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	db.syncCancel = cancel

	db.syncWG.Add(1)
	go func() {
		defer db.syncWG.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				db.runSync(ctx)
			}
		}
	}()
}

func (db *DB) runSync(ctx context.Context) {
	var syncErr error

	if err := db.syncer.Pull(ctx); err != nil {
		syncErr = err
		slog.Warn("turso sync pull failed", "error", err)
	}
	if err := db.syncer.Push(ctx); err != nil {
		syncErr = err
		slog.Warn("turso sync push failed", "error", err)
	}

	db.setSyncStatus(syncErr)
}

func (db *DB) setSyncStatus(err error) {
	db.syncMu.Lock()
	defer db.syncMu.Unlock()
	if err != nil {
		db.lastSyncErr = err
		return
	}
	db.lastSyncOK = time.Now()
	db.lastSyncErr = nil
}

func (db *DB) SyncStatus() (time.Time, error) {
	db.syncMu.Lock()
	defer db.syncMu.Unlock()
	return db.lastSyncOK, db.lastSyncErr
}
