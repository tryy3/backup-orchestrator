package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := New(dbPath)
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	// Verify database file was created.
	_, err = os.Stat(dbPath)
	assert.NoError(t, err)
}

func TestNew_WALMode(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	var journalMode string
	err := db.QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&journalMode)
	require.NoError(t, err)
	assert.Equal(t, "wal", journalMode)
}

func TestNew_BusyTimeout(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	var timeout int
	err := db.QueryRowContext(context.Background(), "PRAGMA busy_timeout").Scan(&timeout)
	require.NoError(t, err)
	assert.Equal(t, 5000, timeout)
}

func TestNew_ForeignKeys(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	var fk int
	err := db.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&fk)
	require.NoError(t, err)
	assert.Equal(t, 1, fk)
}

func TestNew_ConnectionPool(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	stats := db.Stats()
	assert.Equal(t, 25, stats.MaxOpenConnections)
}

func TestNew_Migrations(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	// Verify key tables exist by querying them.
	tables := []string{"agents", "repositories", "settings", "scripts", "backup_plans", "jobs"}
	for _, table := range tables {
		t.Run(table, func(t *testing.T) {
			_, err := db.ExecContext(context.Background(), "SELECT 1 FROM "+table+" LIMIT 0")
			assert.NoError(t, err, "table %s should exist", table)
		})
	}
}

func TestNew_InvalidPath(t *testing.T) {
	t.Parallel()

	db, err := New("/nonexistent/dir/test.db")
	assert.Error(t, err)
	assert.Nil(t, db)
}

func TestDB_Close(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	err := db.Close()
	assert.NoError(t, err)

	// Queries should fail after close.
	_, err = db.ExecContext(context.Background(), "SELECT 1")
	assert.Error(t, err)
}

func TestDB_Close_Idempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := New(dbPath)
	require.NoError(t, err)

	err = db.Close()
	assert.NoError(t, err)

	// Second close should not panic. sql.DB.Close() is idempotent.
	err = db.Close()
	_ = err // May or may not return an error depending on driver.
}

func TestDB_ContextCancellation(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := db.ExecContext(ctx, "SELECT 1")
	assert.Error(t, err)
}

// newTestDB creates an in-memory test database with all migrations applied.
func newTestDB(t *testing.T) *DB {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := New(dbPath)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func TestDB_AppendJobLogs(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()

	// Create an agent first (FK constraint).
	err := db.CreateAgent(ctx, &Agent{
		ID:       "agent-1",
		Name:     "test-agent",
		Hostname: "localhost",
		Status:   "approved",
	})
	require.NoError(t, err)

	// Create a job.
	job := &Job{
		ID:        "test-job-1",
		AgentID:   "agent-1",
		PlanName:  "daily",
		Type:      "backup",
		Trigger:   "manual",
		Status:    "running",
		StartedAt: time.Now().UTC(),
	}
	err = db.CreateJob(ctx, job)
	require.NoError(t, err)

	// Append first batch of logs.
	err = db.AppendJobLogs(ctx, "test-job-1", []LogEntry{
		{Timestamp: "2025-01-01T00:00:00Z", Level: "info", Source: "test", Message: "starting"},
	})
	require.NoError(t, err)

	// Verify.
	j, err := db.GetJob(ctx, "test-job-1")
	require.NoError(t, err)
	assert.Len(t, j.LogEntries, 1)
	assert.Equal(t, "starting", j.LogEntries[0].Message)

	// Append second batch — should be cumulative.
	err = db.AppendJobLogs(ctx, "test-job-1", []LogEntry{
		{Timestamp: "2025-01-01T00:00:01Z", Level: "info", Source: "test", Message: "mid-progress"},
		{Timestamp: "2025-01-01T00:00:02Z", Level: "info", Source: "test", Message: "finishing"},
	})
	require.NoError(t, err)

	j, err = db.GetJob(ctx, "test-job-1")
	require.NoError(t, err)
	assert.Len(t, j.LogEntries, 3)
	assert.Equal(t, "starting", j.LogEntries[0].Message)
	assert.Equal(t, "mid-progress", j.LogEntries[1].Message)
	assert.Equal(t, "finishing", j.LogEntries[2].Message)
}

func TestDB_AppendJobLogs_NonexistentJob(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()

	// Appending to a non-existent job should not return an error.
	err := db.AppendJobLogs(ctx, "nonexistent", []LogEntry{
		{Timestamp: "2025-01-01T00:00:00Z", Level: "info", Source: "test", Message: "ignored"},
	})
	assert.NoError(t, err)
}

func TestDB_AppendJobLogs_EmptyEntries(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()

	err := db.AppendJobLogs(ctx, "any-job", nil)
	assert.NoError(t, err)
}
