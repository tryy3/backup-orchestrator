package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
)

func TestNewAndStop(t *testing.T) {
	s := New(context.Background(), nil, nil)
	s.Start()
	s.Stop()
}

func TestUpdateSchedule_EnabledPlan(t *testing.T) {
	var reported int
	reportFn := func(report *backupv1.JobReport) {
		reported++
	}

	s := New(context.Background(), nil, reportFn)
	defer s.Stop()

	plans := []*backupv1.BackupPlan{
		{Id: "p1", Name: "daily", Enabled: true, Schedule: "0 0 * * *"},
	}

	s.UpdateSchedule(plans, nil, nil)

	s.mu.Lock()
	count := len(s.entryIDs)
	s.mu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 cron entry, got %d", count)
	}
}

func TestUpdateSchedule_DisabledPlan(t *testing.T) {
	s := New(context.Background(), nil, nil)
	defer s.Stop()

	plans := []*backupv1.BackupPlan{
		{Id: "p1", Name: "daily", Enabled: false, Schedule: "0 0 * * *"},
	}

	s.UpdateSchedule(plans, nil, nil)

	s.mu.Lock()
	count := len(s.entryIDs)
	s.mu.Unlock()

	if count != 0 {
		t.Errorf("expected 0 cron entries for disabled plan, got %d", count)
	}
}

func TestUpdateSchedule_NoSchedule(t *testing.T) {
	s := New(context.Background(), nil, nil)
	defer s.Stop()

	plans := []*backupv1.BackupPlan{
		{Id: "p1", Name: "manual-only", Enabled: true, Schedule: ""},
	}

	s.UpdateSchedule(plans, nil, nil)

	s.mu.Lock()
	count := len(s.entryIDs)
	s.mu.Unlock()

	if count != 0 {
		t.Errorf("expected 0 cron entries for no-schedule plan, got %d", count)
	}
}

func TestUpdateSchedule_InvalidCron(t *testing.T) {
	s := New(context.Background(), nil, nil)
	defer s.Stop()

	plans := []*backupv1.BackupPlan{
		{Id: "p1", Name: "bad-cron", Enabled: true, Schedule: "not a cron"},
	}

	s.UpdateSchedule(plans, nil, nil)

	s.mu.Lock()
	count := len(s.entryIDs)
	s.mu.Unlock()

	if count != 0 {
		t.Errorf("expected 0 entries for invalid cron, got %d", count)
	}
}

func TestUpdateSchedule_ReplacesOldEntries(t *testing.T) {
	s := New(context.Background(), nil, nil)
	defer s.Stop()
	s.Start()

	plans1 := []*backupv1.BackupPlan{
		{Id: "p1", Name: "daily", Enabled: true, Schedule: "0 0 * * *"},
		{Id: "p2", Name: "weekly", Enabled: true, Schedule: "0 0 * * 0"},
	}
	s.UpdateSchedule(plans1, nil, nil)

	s.mu.Lock()
	if len(s.entryIDs) != 2 {
		t.Fatalf("first update: expected 2, got %d", len(s.entryIDs))
	}
	s.mu.Unlock()

	plans2 := []*backupv1.BackupPlan{
		{Id: "p3", Name: "hourly", Enabled: true, Schedule: "0 * * * *"},
	}
	s.UpdateSchedule(plans2, nil, nil)

	s.mu.Lock()
	count := len(s.entryIDs)
	s.mu.Unlock()

	if count != 1 {
		t.Errorf("second update: expected 1, got %d", count)
	}
}

func TestJobStatusFunc_Idle(t *testing.T) {
	s := New(context.Background(), nil, nil)
	defer s.Stop()

	fn := s.JobStatusFunc()
	status := fn()
	if status != nil {
		t.Error("expected nil job status when idle")
	}
}

func TestJobStatusFunc_Running(t *testing.T) {
	s := New(context.Background(), nil, nil)
	defer s.Stop()

	s.setCurrentJob("p1", "test-plan")

	fn := s.JobStatusFunc()
	status := fn()
	if status == nil {
		t.Fatal("expected non-nil job status")
		return
	}
	if status.PlanName != "test-plan" {
		t.Errorf("plan name: got %q, want %q", status.PlanName, "test-plan")
	}
	if status.ProgressPercent != -1 {
		t.Errorf("progress: got %v, want -1", status.ProgressPercent)
	}

	s.clearCurrentJob("p1")
	status = fn()
	if status != nil {
		t.Error("expected nil after clearing job")
	}
}

func TestJobStatusFunc_MultiplePlans_ReturnsEarliest(t *testing.T) {
	s := New(context.Background(), nil, nil)
	defer s.Stop()

	// Start p1 first, then p2; the earlier job must be reported in heartbeats.
	s.setCurrentJob("p1", "first-plan")
	time.Sleep(5 * time.Millisecond)
	s.setCurrentJob("p2", "second-plan")

	status := s.JobStatusFunc()()
	if status == nil {
		t.Fatal("expected non-nil job status")
	}
	if status.PlanName != "first-plan" {
		t.Errorf("expected earliest plan name %q, got %q", "first-plan", status.PlanName)
	}

	// Clearing the earliest should fall back to the other running plan.
	s.clearCurrentJob("p1")
	status = s.JobStatusFunc()()
	if status == nil || status.PlanName != "second-plan" {
		t.Errorf("after clearing p1, expected second-plan, got %+v", status)
	}
}

func TestTryStartJob_SamePlanRejected(t *testing.T) {
	s := New(context.Background(), nil, nil)
	defer s.Stop()

	if !s.tryStartJob("p1", "daily") {
		t.Fatal("first tryStartJob should succeed")
	}
	if s.tryStartJob("p1", "daily") {
		t.Error("second tryStartJob for same plan must be rejected")
	}

	// After clearing, it should be accepted again.
	s.clearCurrentJob("p1")
	if !s.tryStartJob("p1", "daily") {
		t.Error("tryStartJob after clear should succeed")
	}
}

func TestTryStartJob_DifferentPlansAllowed(t *testing.T) {
	s := New(context.Background(), nil, nil)
	defer s.Stop()

	if !s.tryStartJob("p1", "daily") {
		t.Fatal("first plan should start")
	}
	if !s.tryStartJob("p2", "weekly") {
		t.Error("second plan (different ID) should start concurrently")
	}
}

func TestTriggerNow_DuplicateEmitsAbortedReport(t *testing.T) {
	var reports []*backupv1.JobReport
	var mu sync.Mutex
	reportFn := func(r *backupv1.JobReport) {
		mu.Lock()
		defer mu.Unlock()
		reports = append(reports, r)
	}

	s := New(context.Background(), nil, reportFn)
	defer s.Stop()

	// Simulate a job already running for this plan.
	s.setCurrentJob("p1", "daily")

	plans := []*backupv1.BackupPlan{
		{Id: "p1", Name: "daily"},
	}
	s.TriggerNow("p1", plans, nil, nil)

	// TriggerNow launches a goroutine; give it a moment to run.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(reports)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(reports) != 1 {
		t.Fatalf("expected 1 aborted report, got %d", len(reports))
	}
	r := reports[0]
	if r.Status != "aborted" {
		t.Errorf("status: got %q, want aborted", r.Status)
	}
	if r.PlanId != "p1" || r.PlanName != "daily" {
		t.Errorf("plan fields mismatch: id=%q name=%q", r.PlanId, r.PlanName)
	}
	if r.Trigger != "manual" {
		t.Errorf("trigger: got %q, want manual", r.Trigger)
	}
	if r.StartedAt == nil || r.FinishedAt == nil {
		t.Error("aborted report should have started_at and finished_at set")
	}
}

func TestTriggerNow_PlanNotFound(t *testing.T) {
	s := New(context.Background(), nil, nil)
	defer s.Stop()

	// Should not panic with nil plans.
	s.TriggerNow("nonexistent", nil, nil, nil)
}

// TestSchedulerUsesUTC verifies that cron entries are scheduled relative to
// UTC, not the host's local timezone. The next-fire time for a schedule that
// runs at the top of every minute must fall within the current UTC minute,
// regardless of what TZ is set on the host.
func TestSchedulerUsesUTC(t *testing.T) {
	s := New(context.Background(), nil, nil)
	defer s.Stop()
	s.Start()

	plans := []*backupv1.BackupPlan{
		{Id: "p1", Name: "every-minute", Enabled: true, Schedule: "* * * * *"},
	}
	s.UpdateSchedule(plans, nil, nil)

	s.mu.Lock()
	entryID, ok := s.entryIDs["p1"]
	s.mu.Unlock()
	if !ok {
		t.Fatal("expected cron entry for plan p1")
	}

	entry := s.cron.Entry(entryID)
	nextUTC := entry.Next

	// The next fire time must be expressed in UTC.
	if nextUTC.Location() != time.UTC {
		t.Errorf("cron next-fire location = %v, want UTC", nextUTC.Location())
	}

	// It must fire within the next minute from now (UTC).
	now := time.Now().UTC()
	if nextUTC.Before(now) || nextUTC.After(now.Add(time.Minute)) {
		t.Errorf("next fire time %v is not within the next UTC minute (now=%v)", nextUTC, now)
	}
}
