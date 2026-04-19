package database

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// SpillItem represents an item that was spilled to SQLite when the in-memory
// outbox queue could not be drained promptly (server unreachable, queue full).
type SpillItem struct {
	ID        string
	Kind      string // "job_report" or "job_event"
	Payload   []byte // protojson-encoded message
	Attempts  int
	LastError string
	CreatedAt string // RFC3339-ish DATETIME string used as a paging cursor
}

// DB wraps the agent's local SQLite database.
type DB struct {
	db *sql.DB
}

// Open creates or opens the agent SQLite database at {dataDir}/state.db,
// enables WAL mode, foreign keys, and recommended SQLite pragmas, and runs migrations.
func Open(dataDir string) (*DB, error) {
	dbPath := filepath.Join(dataDir, "state.db")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Limit to a single connection — SQLite only supports one writer.
	// Pin the idle connection so pragmas set at open time are never lost
	// when database/sql closes and reopens the underlying connection.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxIdleTime(0)

	// Enable WAL mode for better concurrent read performance.
	if _, err := sqlDB.ExecContext(context.Background(), "PRAGMA journal_mode=WAL"); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}

	// Retry on SQLITE_BUSY for up to 5 seconds instead of failing immediately.
	if _, err := sqlDB.ExecContext(context.Background(), "PRAGMA busy_timeout=5000"); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("setting busy timeout: %w", err)
	}

	// NORMAL is the recommended pairing for WAL: durable across app crashes,
	// significantly faster than the default FULL.
	if _, err := sqlDB.ExecContext(context.Background(), "PRAGMA synchronous=NORMAL"); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("setting synchronous mode: %w", err)
	}

	// Enable foreign key enforcement.
	if _, err := sqlDB.ExecContext(context.Background(), "PRAGMA foreign_keys=ON"); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	d := &DB{db: sqlDB}
	if err := d.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return d, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) migrate() error {
	ctx := context.Background()

	// outbox_spill is a single overflow store for the in-memory outbox.
	// It replaces the legacy `buffered_reports` and `local_jobs` tables.
	if _, err := d.db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS outbox_spill (
			id          TEXT PRIMARY KEY,
			kind        TEXT NOT NULL,
			payload     BLOB NOT NULL,
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			attempts    INTEGER NOT NULL DEFAULT 0,
			last_error  TEXT
		)`,
	); err != nil {
		return fmt.Errorf("creating outbox_spill: %w", err)
	}
	if _, err := d.db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_outbox_spill_created_at ON outbox_spill(created_at)`,
	); err != nil {
		return fmt.Errorf("creating outbox_spill index: %w", err)
	}

	// One-time migration from the legacy `buffered_reports` table, if present.
	// Preserves any in-flight reports across the upgrade.
	if exists, err := d.tableExists(ctx, "buffered_reports"); err != nil {
		return err
	} else if exists {
		if _, err := d.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO outbox_spill (id, kind, payload, created_at, attempts, last_error)
			 SELECT id, 'job_report', CAST(payload AS BLOB), created_at, attempts, last_error
			 FROM buffered_reports`,
		); err != nil {
			return fmt.Errorf("migrating buffered_reports: %w", err)
		}
		if _, err := d.db.ExecContext(ctx, `DROP TABLE buffered_reports`); err != nil {
			return fmt.Errorf("dropping buffered_reports: %w", err)
		}
	}

	// `local_jobs` had no production reader and is dropped without migration.
	if _, err := d.db.ExecContext(ctx, `DROP TABLE IF EXISTS local_jobs`); err != nil {
		return fmt.Errorf("dropping local_jobs: %w", err)
	}

	return nil
}

func (d *DB) tableExists(ctx context.Context, name string) (bool, error) {
	var n int
	err := d.db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("checking table %s: %w", name, err)
	}
	return n > 0, nil
}

// SpillEnqueue persists a single outbox item that could not be delivered
// in-memory (server unreachable or in-memory queue full). Existing attempts
// and last_error fields are preserved so an item that has already been tried
// once in memory can be persisted with its retry counter intact.
func (d *DB) SpillEnqueue(ctx context.Context, item SpillItem) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO outbox_spill (id, kind, payload, attempts, last_error)
		 VALUES (?, ?, ?, ?, NULLIF(?, ''))`,
		item.ID, item.Kind, item.Payload, item.Attempts, item.LastError,
	)
	if err != nil {
		return fmt.Errorf("enqueue spill item: %w", err)
	}
	return nil
}

// SpillPage returns up to `limit` items ordered oldest-first. Pass empty
// strings for `afterCreatedAt` and `afterID` to start from the beginning,
// or the last seen row's values to continue paging.
//
// Ordering is `(created_at ASC, id ASC)` so a stable cursor exists even when
// multiple rows share a timestamp.
func (d *DB) SpillPage(ctx context.Context, limit int, afterCreatedAt, afterID string) ([]SpillItem, error) {
	// CAST(created_at AS TEXT) keeps the cursor as a stable string regardless
	// of how the SQLite driver materialises DATETIME columns.
	const cols = `id, kind, payload, attempts, COALESCE(last_error, ''), CAST(created_at AS TEXT)`

	var (
		rows *sql.Rows
		err  error
	)
	if afterID == "" {
		rows, err = d.db.QueryContext(ctx,
			`SELECT `+cols+` FROM outbox_spill
			 ORDER BY created_at ASC, id ASC LIMIT ?`,
			limit,
		)
	} else {
		rows, err = d.db.QueryContext(ctx,
			`SELECT `+cols+` FROM outbox_spill
			 WHERE (CAST(created_at AS TEXT) > ?)
			    OR (CAST(created_at AS TEXT) = ? AND id > ?)
			 ORDER BY created_at ASC, id ASC LIMIT ?`,
			afterCreatedAt, afterCreatedAt, afterID, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("query spill page: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SpillItem
	for rows.Next() {
		var it SpillItem
		if err = rows.Scan(&it.ID, &it.Kind, &it.Payload, &it.Attempts, &it.LastError, &it.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan spill item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// SpillDelete removes a successfully delivered item.
func (d *DB) SpillDelete(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM outbox_spill WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete spill item: %w", err)
	}
	return nil
}

// SpillIncrementAttempts records a delivery failure.
func (d *DB) SpillIncrementAttempts(ctx context.Context, id, lastError string) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE outbox_spill SET attempts = attempts + 1, last_error = ? WHERE id = ?`,
		lastError, id,
	)
	if err != nil {
		return fmt.Errorf("increment spill attempts: %w", err)
	}
	return nil
}

// SpillCount returns the number of items currently in the spill table.
func (d *DB) SpillCount(ctx context.Context) (int, error) {
	var n int
	if err := d.db.QueryRowContext(ctx, `SELECT count(*) FROM outbox_spill`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count spill: %w", err)
	}
	return n, nil
}

// SpillPruneByAge deletes rows older than the given cutoff timestamp
// (formatted to match the SQLite DATETIME column). Returns the number of
// rows removed.
func (d *DB) SpillPruneByAge(ctx context.Context, cutoff string) (int64, error) {
	res, err := d.db.ExecContext(ctx,
		`DELETE FROM outbox_spill WHERE created_at < ?`, cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("prune spill by age: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// SpillPruneByCount keeps only the newest `keep` rows, deleting the rest.
// Returns the number of rows removed.
func (d *DB) SpillPruneByCount(ctx context.Context, keep int) (int64, error) {
	if keep < 0 {
		keep = 0
	}
	res, err := d.db.ExecContext(ctx,
		`DELETE FROM outbox_spill
		 WHERE id IN (
		   SELECT id FROM outbox_spill
		   ORDER BY created_at DESC, id DESC
		   LIMIT -1 OFFSET ?
		 )`,
		keep,
	)
	if err != nil {
		return 0, fmt.Errorf("prune spill by count: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// SpillDeleteOldest removes the oldest `n` rows. Used when the in-memory
// outbox needs to spill but the spill table is already at capacity.
// Returns the number of rows actually removed.
func (d *DB) SpillDeleteOldest(ctx context.Context, n int) (int64, error) {
	if n <= 0 {
		return 0, nil
	}
	res, err := d.db.ExecContext(ctx,
		`DELETE FROM outbox_spill
		 WHERE id IN (
		   SELECT id FROM outbox_spill
		   ORDER BY created_at ASC, id ASC
		   LIMIT ?
		 )`,
		n,
	)
	if err != nil {
		return 0, fmt.Errorf("drop oldest spill rows: %w", err)
	}
	rows, _ := res.RowsAffected()
	return rows, nil
}

// SpillCheckpoint runs `PRAGMA wal_checkpoint(TRUNCATE)` to reclaim disk
// after large prunes. Best-effort.
func (d *DB) SpillCheckpoint(ctx context.Context) error {
	if _, err := d.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("wal checkpoint: %w", err)
	}
	return nil
}
