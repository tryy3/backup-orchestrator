package database

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

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
	return fmt.Errorf("turso-sync: remote unreachable and local database is empty: %w", pullErr)
}

func localDBReady(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

func (db *DB) startSyncLoop(interval time.Duration) {
	if db.syncer == nil || interval <= 0 {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	db.syncCancel = cancel

	go func() {
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
