package scheduler

import (
	"context"
	"log/slog"
	"sync"

	"github.com/robfig/cron/v3"
	"github.com/tryy3/backup-orchestrator/agent/internal/executor"
	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
)

// ReportFunc is called after a backup job completes to report the result.
type ReportFunc func(report *backupv1.JobReport)

// Scheduler manages cron-based execution of backup plans.
type Scheduler struct {
	cron     *cron.Cron
	executor *executor.JobOrchestrator
	reporter ReportFunc
	mu       sync.Mutex
	entryIDs map[string]cron.EntryID // plan_id -> entry
}

// New creates a new Scheduler.
func New(exec *executor.JobOrchestrator, reportFn ReportFunc) *Scheduler {
	return &Scheduler{
		cron:     cron.New(),
		executor: exec,
		reporter: reportFn,
		entryIDs: make(map[string]cron.EntryID),
	}
}

// Start begins the cron scheduler.
func (s *Scheduler) Start() {
	s.cron.Start()
}

// Stop stops the cron scheduler and waits for running jobs to finish.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

// UpdateSchedule removes all existing cron entries and adds new ones for each
// enabled plan. Each cron callback runs the backup job and reports the result.
func (s *Scheduler) UpdateSchedule(
	plans []*backupv1.BackupPlan,
	repos []*backupv1.Repository,
	defaultRetention *backupv1.RetentionPolicy,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove all existing entries.
	for planID, entryID := range s.entryIDs {
		s.cron.Remove(entryID)
		delete(s.entryIDs, planID)
	}

	// Add new entries for each enabled plan.
	for _, plan := range plans {
		if !plan.GetEnabled() {
			slog.Info("skipping disabled plan", "source", "scheduler", "plan", plan.GetName())
			continue
		}

		schedule := plan.GetSchedule()
		if schedule == "" {
			slog.Info("plan has no schedule, skipping", "source", "scheduler", "plan", plan.GetName())
			continue
		}

		// Capture loop variables for the closure.
		p := plan
		r := repos
		dr := defaultRetention

		entryID, err := s.cron.AddFunc(schedule, func() {
			slog.Info("triggered scheduled backup", "source", "scheduler", "plan", p.GetName())
			report := s.executor.ExecuteBackupJob(context.Background(), p, r, dr, "scheduled")
			if s.reporter != nil {
				s.reporter(report)
			}
		})
		if err != nil {
			slog.Error("error adding schedule for plan", "source", "scheduler", "plan", plan.GetName(), "error", err)
			continue
		}

		s.entryIDs[plan.GetId()] = entryID
		slog.Info("scheduled plan", "source", "scheduler", "plan", plan.GetName(), "cron", schedule)
	}
}

// TriggerNow finds the plan by ID and executes it immediately in a goroutine.
func (s *Scheduler) TriggerNow(
	planID string,
	plans []*backupv1.BackupPlan,
	repos []*backupv1.Repository,
	defaultRetention *backupv1.RetentionPolicy,
) {
	var targetPlan *backupv1.BackupPlan
	for _, p := range plans {
		if p.GetId() == planID {
			targetPlan = p
			break
		}
	}

	if targetPlan == nil {
		slog.Warn("plan not found for manual trigger", "source", "scheduler", "plan_id", planID)
		return
	}

	go func() {
		slog.Info("manual trigger for plan", "source", "scheduler", "plan", targetPlan.GetName())
		report := s.executor.ExecuteBackupJob(context.Background(), targetPlan, repos, defaultRetention, "manual")
		if s.reporter != nil {
			s.reporter(report)
		}
	}()
}
