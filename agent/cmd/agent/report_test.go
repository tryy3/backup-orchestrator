package main

import (
	"context"
	"testing"
	"time"

	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
)

// hangingReporter blocks ReportJob until its context is cancelled, simulating
// a server that is unreachable during agent shutdown.
type hangingReporter struct{}

func (h *hangingReporter) ReportJob(ctx context.Context, _ *backupv1.JobReport) error {
	<-ctx.Done()
	return ctx.Err()
}

// noopBufferer silently accepts buffered reports.
type noopBufferer struct{}

func (n *noopBufferer) BufferReport(_ context.Context, _ *backupv1.JobReport) error { return nil }

// TestDeliverReport_CancelledParentReturnsPromptly verifies that deliverReport
// returns well within the 10 s delivery timeout when the parent context is
// cancelled, rather than waiting for the full delivery timeout to expire.
func TestDeliverReport_CancelledParentReturnsPromptly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel the parent context immediately — simulates SIGTERM during shutdown.
	cancel()

	report := &backupv1.JobReport{JobId: "j1", Status: "success"}

	start := time.Now()
	deliverReport(ctx, &hangingReporter{}, &noopBufferer{}, report)
	elapsed := time.Since(start)

	const maxAllowed = 200 * time.Millisecond
	if elapsed > maxAllowed {
		t.Errorf("deliverReport took %v; want < %v (delivery timeout must not fire on shutdown)", elapsed, maxAllowed)
	}
}
