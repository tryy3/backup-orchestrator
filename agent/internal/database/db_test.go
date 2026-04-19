package database

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpen_WALMode(t *testing.T) {
	db := openTestDB(t)
	var journalMode string
	if err := db.db.QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode: got %q, want %q", journalMode, "wal")
	}
}

func TestOpen_BusyTimeout(t *testing.T) {
	db := openTestDB(t)
	var timeout int
	if err := db.db.QueryRowContext(context.Background(), "PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if timeout != 5000 {
		t.Errorf("busy_timeout: got %d, want 5000", timeout)
	}
}

func TestOpen_SynchronousNormal(t *testing.T) {
	db := openTestDB(t)
	var synchronous int
	if err := db.db.QueryRowContext(context.Background(), "PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatalf("PRAGMA synchronous: %v", err)
	}
	if synchronous != 1 {
		t.Errorf("synchronous: got %d, want 1 (NORMAL)", synchronous)
	}
}

func TestOpen_ForeignKeys(t *testing.T) {
	db := openTestDB(t)
	var fk int
	if err := db.db.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys: got %d, want 1", fk)
	}
}

func TestOpen_ConnectionPool(t *testing.T) {
	db := openTestDB(t)
	stats := db.db.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Errorf("MaxOpenConnections: got %d, want 1", stats.MaxOpenConnections)
	}
}

func TestMigrate_OutboxSpillExists(t *testing.T) {
	db := openTestDB(t)
	var n int
	if err := db.db.QueryRowContext(context.Background(),
		"SELECT count(*) FROM outbox_spill").Scan(&n); err != nil {
		t.Fatalf("outbox_spill table missing: %v", err)
	}
	if n != 0 {
		t.Errorf("expected empty outbox_spill, got %d rows", n)
	}
}

func TestMigrate_LegacyTablesDropped(t *testing.T) {
	db := openTestDB(t)
	for _, name := range []string{"buffered_reports", "local_jobs"} {
		exists, err := db.tableExists(context.Background(), name)
		if err != nil {
			t.Fatalf("tableExists(%s): %v", name, err)
		}
		if exists {
			t.Errorf("legacy table %s should have been dropped", name)
		}
	}
}

func TestMigrate_PreservesLegacyBufferedReports(t *testing.T) {
	dir := t.TempDir()

	db1, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, execErr := db1.db.ExecContext(context.Background(),
		`CREATE TABLE buffered_reports (
			id          TEXT PRIMARY KEY,
			payload     TEXT NOT NULL,
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			attempts    INTEGER NOT NULL DEFAULT 0,
			last_error  TEXT
		)`); execErr != nil {
		t.Fatalf("recreate legacy table: %v", execErr)
	}
	if _, execErr := db1.db.ExecContext(context.Background(),
		`INSERT INTO buffered_reports (id, payload, attempts, last_error)
		 VALUES ('a', '{"job_id":"old"}', 1, 'tmo'), ('b', '{"job_id":"old2"}', 0, NULL)`,
	); execErr != nil {
		t.Fatalf("seed legacy rows: %v", execErr)
	}
	if closeErr := db1.Close(); closeErr != nil {
		t.Fatalf("close: %v", closeErr)
	}

	db2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()

	exists, err := db2.tableExists(context.Background(), "buffered_reports")
	if err != nil {
		t.Fatalf("tableExists: %v", err)
	}
	if exists {
		t.Errorf("buffered_reports should be dropped after migration")
	}

	items, err := db2.SpillPage(context.Background(), 100, "", "")
	if err != nil {
		t.Fatalf("SpillPage: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 migrated rows, got %d", len(items))
	}
	for _, it := range items {
		if it.Kind != "job_report" {
			t.Errorf("migrated row kind: got %q, want job_report", it.Kind)
		}
	}
}

func TestSpill_EnqueuePageDelete(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	for i, id := range []string{"r1", "r2", "r3"} {
		if err := db.SpillEnqueue(ctx, SpillItem{
			ID:      id,
			Kind:    "job_report",
			Payload: []byte(fmt.Sprintf(`{"i":%d}`, i)),
		}); err != nil {
			t.Fatalf("SpillEnqueue %s: %v", id, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	items, err := db.SpillPage(ctx, 10, "", "")
	if err != nil {
		t.Fatalf("SpillPage: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0].ID != "r1" || items[2].ID != "r3" {
		t.Errorf("ordering: got %s,%s,%s", items[0].ID, items[1].ID, items[2].ID)
	}

	if delErr := db.SpillDelete(ctx, "r2"); delErr != nil {
		t.Fatalf("SpillDelete: %v", delErr)
	}
	n, err := db.SpillCount(ctx)
	if err != nil {
		t.Fatalf("SpillCount: %v", err)
	}
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestSpill_PageCursor(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	for i := 0; i < 5; i++ {
		if err := db.SpillEnqueue(ctx, SpillItem{
			ID:      fmt.Sprintf("id-%d", i),
			Kind:    "job_report",
			Payload: []byte("{}"),
		}); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	first, err := db.SpillPage(ctx, 2, "", "")
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("page 1 size: got %d", len(first))
	}

	second, err := db.SpillPage(ctx, 2, first[1].CreatedAt, first[1].ID)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(second) != 2 {
		t.Fatalf("page 2 size: got %d", len(second))
	}
	if second[0].ID == first[1].ID {
		t.Errorf("cursor included previous last row")
	}

	third, err := db.SpillPage(ctx, 2, second[1].CreatedAt, second[1].ID)
	if err != nil {
		t.Fatalf("page 3: %v", err)
	}
	if len(third) != 1 {
		t.Errorf("page 3 size: got %d, want 1", len(third))
	}
}

func TestSpill_IncrementAttempts(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	if err := db.SpillEnqueue(ctx, SpillItem{ID: "r1", Kind: "job_report", Payload: []byte("{}")}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := db.SpillIncrementAttempts(ctx, "r1", "timeout"); err != nil {
		t.Fatalf("inc 1: %v", err)
	}
	if err := db.SpillIncrementAttempts(ctx, "r1", "refused"); err != nil {
		t.Fatalf("inc 2: %v", err)
	}

	items, err := db.SpillPage(ctx, 10, "", "")
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if items[0].Attempts != 2 {
		t.Errorf("attempts: got %d, want 2", items[0].Attempts)
	}
	if items[0].LastError != "refused" {
		t.Errorf("last_error: got %q", items[0].LastError)
	}
}

func TestSpill_PruneByCount(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	for i := 0; i < 10; i++ {
		if err := db.SpillEnqueue(ctx, SpillItem{
			ID:      fmt.Sprintf("id-%d", i),
			Kind:    "job_report",
			Payload: []byte("{}"),
		}); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	deleted, err := db.SpillPruneByCount(ctx, 3)
	if err != nil {
		t.Fatalf("PruneByCount: %v", err)
	}
	if deleted != 7 {
		t.Errorf("deleted: got %d, want 7", deleted)
	}

	items, err := db.SpillPage(ctx, 100, "", "")
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("remaining: got %d, want 3", len(items))
	}
	if items[0].ID != "id-7" || items[2].ID != "id-9" {
		t.Errorf("retained wrong rows: %s,%s,%s", items[0].ID, items[1].ID, items[2].ID)
	}
}

func TestSpill_PruneByAge(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	if err := db.SpillEnqueue(ctx, SpillItem{ID: "old", Kind: "k", Payload: []byte("{}")}); err != nil {
		t.Fatalf("enqueue old: %v", err)
	}
	if _, err := db.db.ExecContext(ctx,
		`UPDATE outbox_spill SET created_at = datetime('now', '-30 days') WHERE id = 'old'`,
	); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if err := db.SpillEnqueue(ctx, SpillItem{ID: "fresh", Kind: "k", Payload: []byte("{}")}); err != nil {
		t.Fatalf("enqueue fresh: %v", err)
	}

	cutoff := time.Now().AddDate(0, 0, -7).UTC().Format("2006-01-02 15:04:05")
	deleted, err := db.SpillPruneByAge(ctx, cutoff)
	if err != nil {
		t.Fatalf("PruneByAge: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted: got %d, want 1", deleted)
	}

	items, err := db.SpillPage(ctx, 10, "", "")
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if len(items) != 1 || items[0].ID != "fresh" {
		t.Errorf("retained wrong rows: %v", items)
	}
}

func TestSpill_DeleteOldest(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	for i := 0; i < 5; i++ {
		if err := db.SpillEnqueue(ctx, SpillItem{
			ID:      fmt.Sprintf("id-%d", i),
			Kind:    "k",
			Payload: []byte("{}"),
		}); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	n, err := db.SpillDeleteOldest(ctx, 2)
	if err != nil {
		t.Fatalf("DeleteOldest: %v", err)
	}
	if n != 2 {
		t.Errorf("deleted: got %d, want 2", n)
	}

	items, err := db.SpillPage(ctx, 10, "", "")
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if len(items) != 3 || items[0].ID != "id-2" {
		t.Errorf("after delete oldest: %v", items)
	}
}

func TestSpill_PageEmpty(t *testing.T) {
	db := openTestDB(t)
	items, err := db.SpillPage(t.Context(), 10, "", "")
	if err != nil {
		t.Fatalf("SpillPage: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty, got %d", len(items))
	}
}
