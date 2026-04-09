package scheduler

import (
	"context"
	"testing"

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

	s.setCurrentJob("test-plan")

	fn := s.JobStatusFunc()
	status := fn()
	if status == nil {
		t.Fatal("expected non-nil job status")
	}
	if status.PlanName != "test-plan" {
		t.Errorf("plan name: got %q, want %q", status.PlanName, "test-plan")
	}
	if status.ProgressPercent != -1 {
		t.Errorf("progress: got %v, want -1", status.ProgressPercent)
	}

	s.clearCurrentJob()
	status = fn()
	if status != nil {
		t.Error("expected nil after clearing job")
	}
}

func TestTriggerNow_PlanNotFound(t *testing.T) {
	s := New(context.Background(), nil, nil)
	defer s.Stop()

	// Should not panic with nil plans.
	s.TriggerNow("nonexistent", nil, nil, nil)
}
