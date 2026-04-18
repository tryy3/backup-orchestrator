package reporter

import (
	"context"
	"testing"
	"time"

	"github.com/tryy3/backup-orchestrator/agent/internal/database"
	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
)

// slowReporter blocks inside ReportJob until unblocked, allowing tests to
// verify that a concurrent flush is skipped rather than serialised.
type slowReporter struct {
	started chan struct{} // closed when ReportJob is first entered
	blockCh chan struct{} // closed to unblock ReportJob
}

func (s *slowReporter) ReportJob(_ context.Context, _ *backupv1.JobReport) error {
	close(s.started) // signal that the RPC has started
	<-s.blockCh      // block until the test releases us
	return nil
}

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

// TestFlush_ConcurrentFlushSkipped verifies that a second concurrent call to
// flush returns immediately instead of blocking behind an in-flight flush.
func TestFlush_ConcurrentFlushSkipped(t *testing.T) {
	db := openTestDB(t)

	slow := &slowReporter{
		started: make(chan struct{}),
		blockCh: make(chan struct{}),
	}

	r := New(db, nil, 0)
	r.grpc = slow // inject slow reporter directly (same package)

	// Buffer one report so flush has work to do.
	if err := r.BufferReport(ctx, &backupv1.JobReport{JobId: "j1"}); err != nil {
		t.Fatalf("BufferReport: %v", err)
	}

	ctx := context.Background()

	// First flush — will block inside ReportJob.
	go r.flush(ctx)

	// Wait until the first flush has entered ReportJob.
	select {
	case <-slow.started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first flush to start ReportJob")
	}

	// Second flush must return immediately without blocking.
	secondDone := make(chan struct{})
	go func() {
		r.flush(ctx)
		close(secondDone)
	}()

	select {
	case <-secondDone:
		// correct: second flush skipped
	case <-time.After(time.Second):
		t.Fatal("second flush blocked instead of returning immediately")
	}

	// Unblock the first flush so the goroutine can finish cleanly.
	close(slow.blockCh)
}
