package database

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// BufferedReport represents an unsent job report stored locally.
type BufferedReport struct {
	ID        string
	Payload   string
	Attempts  int
	LastError string
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
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS buffered_reports (
			id          TEXT PRIMARY KEY,
			payload     TEXT NOT NULL,
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			attempts    INTEGER NOT NULL DEFAULT 0,
			last_error  TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS local_jobs (
			id          TEXT PRIMARY KEY,
			plan_name   TEXT NOT NULL,
			type        TEXT NOT NULL,
			status      TEXT NOT NULL,
			started_at  DATETIME NOT NULL,
			finished_at DATETIME,
			log_tail    TEXT,
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_local_jobs_started_at ON local_jobs(started_at)`,
	}

	for _, m := range migrations {
		if _, err := d.db.ExecContext(context.Background(), m); err != nil {
			return fmt.Errorf("executing migration: %w", err)
		}
	}
	return nil
}

// InsertBufferedReport stores a job report for later delivery.
func (d *DB) InsertBufferedReport(id, payload string) error {
	_, err := d.db.ExecContext(context.Background(),
		"INSERT INTO buffered_reports (id, payload) VALUES (?, ?)",
		id, payload,
	)
	if err != nil {
		return fmt.Errorf("inserting buffered report: %w", err)
	}
	return nil
}

// ListPendingReports returns all unsent buffered reports.
func (d *DB) ListPendingReports() ([]BufferedReport, error) {
	rows, err := d.db.QueryContext(context.Background(),
		"SELECT id, payload, attempts, COALESCE(last_error, '') FROM buffered_reports ORDER BY created_at ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("querying buffered reports: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var reports []BufferedReport
	for rows.Next() {
		var r BufferedReport
		if err := rows.Scan(&r.ID, &r.Payload, &r.Attempts, &r.LastError); err != nil {
			return nil, fmt.Errorf("scanning buffered report: %w", err)
		}
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

// DeleteReport removes a successfully sent report.
func (d *DB) DeleteReport(id string) error {
	_, err := d.db.ExecContext(context.Background(), "DELETE FROM buffered_reports WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting report: %w", err)
	}
	return nil
}

// IncrementAttempts increments the attempt count and records the last error.
func (d *DB) IncrementAttempts(id, lastError string) error {
	_, err := d.db.ExecContext(context.Background(),
		"UPDATE buffered_reports SET attempts = attempts + 1, last_error = ? WHERE id = ?",
		lastError, id,
	)
	if err != nil {
		return fmt.Errorf("incrementing attempts: %w", err)
	}
	return nil
}

// InsertLocalJob records a job execution in local history.
func (d *DB) InsertLocalJob(id, planName, jobType, status, startedAt, finishedAt, logTail string) error {
	_, err := d.db.ExecContext(context.Background(),
		`INSERT INTO local_jobs (id, plan_name, type, status, started_at, finished_at, log_tail)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, planName, jobType, status, startedAt, finishedAt, logTail,
	)
	if err != nil {
		return fmt.Errorf("inserting local job: %w", err)
	}
	return nil
}

// LocalJob represents a job from local history.
type LocalJob struct {
	ID         string
	PlanName   string
	Type       string
	Status     string
	StartedAt  string
	FinishedAt string
	LogTail    string
}

// ListLocalJobs returns recent local job records.
func (d *DB) ListLocalJobs(limit, offset int) ([]LocalJob, error) {
	rows, err := d.db.QueryContext(context.Background(),
		`SELECT id, plan_name, type, status, started_at, COALESCE(finished_at, ''), COALESCE(log_tail, '')
		 FROM local_jobs ORDER BY started_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("querying local jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var jobs []LocalJob
	for rows.Next() {
		var j LocalJob
		if err := rows.Scan(&j.ID, &j.PlanName, &j.Type, &j.Status, &j.StartedAt, &j.FinishedAt, &j.LogTail); err != nil {
			return nil, fmt.Errorf("scanning local job: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
