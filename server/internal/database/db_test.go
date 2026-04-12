package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := New(dbPath, nil)
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

	db, err := New("/nonexistent/dir/test.db", nil)
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

	db, err := New(dbPath, nil)
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

	db, err := New(dbPath, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

// newTestDBWithKey creates a test database with encryption enabled.
func newTestDBWithKey(t *testing.T) *DB {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	key := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	db, err := New(dbPath, key)
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

func TestDB_EncryptionRoundTrip_Repository(t *testing.T) {
	t.Parallel()

	db := newTestDBWithKey(t)
	ctx := context.Background()

	repo := &Repository{
		Name:     "encrypted-repo",
		Scope:    "global",
		Type:     "local",
		Path:     "local:/tmp/backup",
		Password: "super-secret",
	}
	err := db.CreateRepository(ctx, repo)
	require.NoError(t, err)

	// Read back — should get plaintext.
	got, err := db.GetRepository(ctx, repo.ID)
	require.NoError(t, err)
	assert.Equal(t, "super-secret", got.Password)

	// Verify the raw DB value is encrypted.
	var raw string
	err = db.QueryRowContext(ctx, "SELECT password FROM repositories WHERE id = ?", repo.ID).Scan(&raw)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(raw, "enc:"), "raw password should be encrypted, got: %s", raw)
}

func TestDB_EncryptionRoundTrip_Agent(t *testing.T) {
	t.Parallel()

	db := newTestDBWithKey(t)
	ctx := context.Background()

	rclone := "[remote]\ntype = s3\n"
	err := db.CreateAgent(ctx, &Agent{
		ID:           "agent-enc",
		Name:         "enc-agent",
		Hostname:     "localhost",
		Status:       "approved",
		RcloneConfig: &rclone,
	})
	require.NoError(t, err)

	// Read back — should get plaintext.
	agent, err := db.GetAgent(ctx, "agent-enc")
	require.NoError(t, err)
	require.NotNil(t, agent.RcloneConfig)
	assert.Equal(t, rclone, *agent.RcloneConfig)

	// Verify the raw DB value is encrypted.
	var raw string
	err = db.QueryRowContext(ctx, "SELECT rclone_config FROM agents WHERE id = ?", "agent-enc").Scan(&raw)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(raw, "enc:"), "raw rclone_config should be encrypted, got: %s", raw)
}

func TestDB_EncryptionMigration(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// First, create DB without encryption and insert plaintext values.
	db1, err := New(dbPath, nil)
	require.NoError(t, err)

	ctx := context.Background()
	repo := &Repository{
		Name:     "plain-repo",
		Scope:    "global",
		Type:     "local",
		Path:     "local:/tmp",
		Password: "plaintext-pw",
	}
	err = db1.CreateRepository(ctx, repo)
	require.NoError(t, err)

	rclone := "[remote]\ntype = b2\n"
	err = db1.CreateAgent(ctx, &Agent{
		ID:           "agent-plain",
		Name:         "plain-agent",
		Hostname:     "host",
		Status:       "approved",
		RcloneConfig: &rclone,
	})
	require.NoError(t, err)

	db1.Close()

	// Re-open with encryption key — migration should encrypt existing values.
	key := []byte("0123456789abcdef0123456789abcdef")
	db2, err := New(dbPath, key)
	require.NoError(t, err)
	defer db2.Close()

	// Verify raw values are now encrypted.
	var rawPw string
	err = db2.QueryRowContext(ctx, "SELECT password FROM repositories WHERE id = ?", repo.ID).Scan(&rawPw)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(rawPw, "enc:"), "password should be encrypted after migration")

	var rawRclone string
	err = db2.QueryRowContext(ctx, "SELECT rclone_config FROM agents WHERE id = ?", "agent-plain").Scan(&rawRclone)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(rawRclone, "enc:"), "rclone_config should be encrypted after migration")

	// Verify decrypted reads still work.
	gotRepo, err := db2.GetRepository(ctx, repo.ID)
	require.NoError(t, err)
	assert.Equal(t, "plaintext-pw", gotRepo.Password)

	gotAgent, err := db2.GetAgent(ctx, "agent-plain")
	require.NoError(t, err)
	require.NotNil(t, gotAgent.RcloneConfig)
	assert.Equal(t, rclone, *gotAgent.RcloneConfig)
}
