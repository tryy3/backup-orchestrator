package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/tryy3/backup-orchestrator/agent/internal/executor"
	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
	"github.com/tryy3/backup-orchestrator/agent/internal/grpcclient"
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

	ctx    context.Context
	cancel context.CancelFunc

	jobMu      sync.RWMutex
	currentJob *grpcclient.JobStatus
}

// New creates a new Scheduler.
func New(ctx context.Context, exec *executor.JobOrchestrator, reportFn ReportFunc) *Scheduler {
	ctx, cancel := context.WithCancel(ctx)
	return &Scheduler{
		cron:     cron.New(),
		executor: exec,
		reporter: reportFn,
		entryIDs: make(map[string]cron.EntryID),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins the cron scheduler.
func (s *Scheduler) Start() {
	s.cron.Start()
}

// Stop cancels running jobs and stops the cron scheduler.
func (s *Scheduler) Stop() {
	s.cancel()
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
			s.setCurrentJob(p.GetName())
			report := s.executor.ExecuteBackupJob(s.ctx, p, r, dr, "scheduled")
			s.clearCurrentJob()
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
		s.setCurrentJob(targetPlan.GetName())
		report := s.executor.ExecuteBackupJob(s.ctx, targetPlan, repos, defaultRetention, "manual")
		s.clearCurrentJob()
		if s.reporter != nil {
			s.reporter(report)
		}
	}()
}

func (s *Scheduler) setCurrentJob(planName string) {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	s.currentJob = &grpcclient.JobStatus{
		PlanName:        planName,
		StartedAt:       time.Now(),
		ProgressPercent: -1, // unknown
	}
}

func (s *Scheduler) clearCurrentJob() {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	s.currentJob = nil
}

// JobStatusFunc returns a function compatible with grpcclient.JobStatusFunc
// that reports the currently running job, or nil if idle.
func (s *Scheduler) JobStatusFunc() grpcclient.JobStatusFunc {
	return func() *grpcclient.JobStatus {
		s.jobMu.RLock()
		defer s.jobMu.RUnlock()
		return s.currentJob
	}
}
