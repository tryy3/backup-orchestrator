package reporter

import (
	"testing"

	"github.com/tryy3/backup-orchestrator/agent/internal/database"
	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
)

func openTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestBufferReport(t *testing.T) {
	db := openTestDB(t)
	r := New(db, nil, 0)
	ctx := t.Context()

	report := &backupv1.JobReport{
		JobId:   "j1",
		PlanId:  "p1",
		AgentId: "a1",
		Status:  "success",
	}

	if err := r.BufferReport(ctx, report); err != nil {
		t.Fatalf("BufferReport: %v", err)
	}

	reports, err := db.ListPendingReports(ctx)
	if err != nil {
		t.Fatalf("ListPendingReports: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 buffered report, got %d", len(reports))
	}
	if reports[0].Payload == "" {
		t.Error("expected non-empty payload")
	}
}

func TestBufferReport_Multiple(t *testing.T) {
	db := openTestDB(t)
	r := New(db, nil, 0)
	ctx := t.Context()

	for i := 0; i < 3; i++ {
		if err := r.BufferReport(ctx, &backupv1.JobReport{JobId: "j" + string(rune('1'+i))}); err != nil {
			t.Fatalf("BufferReport %d: %v", i, err)
		}
	}

	reports, err := db.ListPendingReports(ctx)
	if err != nil {
		t.Fatalf("ListPendingReports: %v", err)
	}
	if len(reports) != 3 {
		t.Errorf("expected 3, got %d", len(reports))
	}
}

func TestFlushNow_NonBlocking(t *testing.T) {
	db := openTestDB(t)
	r := New(db, nil, 0)

	// Should not panic or block even without a running flush loop.
	r.FlushNow()
	r.FlushNow()
}

func TestMaxFlushAttempts(t *testing.T) {
	if maxFlushAttempts != 10 {
		t.Errorf("maxFlushAttempts: got %d, want 10", maxFlushAttempts)
	}
}
