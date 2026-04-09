package database

import (
	"context"
	"testing"
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

func TestOpenAndMigrate(t *testing.T) {
	db := openTestDB(t)
	// Tables should exist after Open.
	var count int
	if err := db.db.QueryRowContext(context.Background(), "SELECT count(*) FROM buffered_reports").Scan(&count); err != nil {
		t.Fatalf("buffered_reports table missing: %v", err)
	}
	if err := db.db.QueryRowContext(context.Background(), "SELECT count(*) FROM local_jobs").Scan(&count); err != nil {
		t.Fatalf("local_jobs table missing: %v", err)
	}
}

func TestBufferedReports_InsertListDelete(t *testing.T) {
	db := openTestDB(t)

	if err := db.InsertBufferedReport("r1", `{"job_id":"j1"}`); err != nil {
		t.Fatalf("InsertBufferedReport: %v", err)
	}
	if err := db.InsertBufferedReport("r2", `{"job_id":"j2"}`); err != nil {
		t.Fatalf("InsertBufferedReport: %v", err)
	}

	reports, err := db.ListPendingReports()
	if err != nil {
		t.Fatalf("ListPendingReports: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}
	if reports[0].ID != "r1" || reports[1].ID != "r2" {
		t.Errorf("unexpected report order: %v", reports)
	}

	if err = db.DeleteReport("r1"); err != nil {
		t.Fatalf("DeleteReport: %v", err)
	}

	reports, err = db.ListPendingReports()
	if err != nil {
		t.Fatalf("ListPendingReports: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report after delete, got %d", len(reports))
	}
}

func TestBufferedReports_IncrementAttempts(t *testing.T) {
	db := openTestDB(t)

	if err := db.InsertBufferedReport("r1", `{}`); err != nil {
		t.Fatalf("InsertBufferedReport: %v", err)
	}

	if err := db.IncrementAttempts("r1", "timeout"); err != nil {
		t.Fatalf("IncrementAttempts: %v", err)
	}
	if err := db.IncrementAttempts("r1", "connection refused"); err != nil {
		t.Fatalf("IncrementAttempts: %v", err)
	}

	reports, err := db.ListPendingReports()
	if err != nil {
		t.Fatalf("ListPendingReports: %v", err)
	}
	if reports[0].Attempts != 2 {
		t.Errorf("attempts: got %d, want 2", reports[0].Attempts)
	}
	if reports[0].LastError != "connection refused" {
		t.Errorf("last_error: got %q, want %q", reports[0].LastError, "connection refused")
	}
}

func TestLocalJobs_InsertAndList(t *testing.T) {
	db := openTestDB(t)

	if err := db.InsertLocalJob("j1", "daily", "backup", "success", "2025-01-01T00:00:00Z", "2025-01-01T00:05:00Z", "log1"); err != nil {
		t.Fatalf("InsertLocalJob: %v", err)
	}
	if err := db.InsertLocalJob("j2", "weekly", "backup", "failed", "2025-01-02T00:00:00Z", "2025-01-02T00:10:00Z", "log2"); err != nil {
		t.Fatalf("InsertLocalJob: %v", err)
	}

	jobs, err := db.ListLocalJobs(10, 0)
	if err != nil {
		t.Fatalf("ListLocalJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	// Ordered by started_at DESC, so j2 first.
	if jobs[0].ID != "j2" {
		t.Errorf("first job: got %q, want j2", jobs[0].ID)
	}
	if jobs[1].PlanName != "daily" {
		t.Errorf("second job plan: got %q, want %q", jobs[1].PlanName, "daily")
	}
}

func TestLocalJobs_Pagination(t *testing.T) {
	db := openTestDB(t)

	for i := 0; i < 5; i++ {
		id := "j" + string(rune('0'+i))
		ts := "2025-01-0" + string(rune('1'+i)) + "T00:00:00Z"
		if err := db.InsertLocalJob(id, "plan", "backup", "success", ts, ts, ""); err != nil {
			t.Fatalf("InsertLocalJob: %v", err)
		}
	}

	jobs, err := db.ListLocalJobs(2, 0)
	if err != nil {
		t.Fatalf("ListLocalJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("limit 2: got %d", len(jobs))
	}

	jobs, err = db.ListLocalJobs(10, 3)
	if err != nil {
		t.Fatalf("ListLocalJobs offset: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("offset 3: got %d, want 2", len(jobs))
	}
}

func TestListPendingReports_Empty(t *testing.T) {
	db := openTestDB(t)
	reports, err := db.ListPendingReports()
	if err != nil {
		t.Fatalf("ListPendingReports: %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("expected empty, got %d", len(reports))
	}
}
