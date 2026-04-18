package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"github.com/tryy3/backup-orchestrator/agent/internal/executor"
	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
	"github.com/tryy3/backup-orchestrator/agent/internal/grpcclient"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ReportFunc is called after a backup job completes to report the result.
type ReportFunc func(report *backupv1.JobReport)

// Scheduler manages cron-based execution of backup plans.
//
// Concurrency policy:
//   - Different plans may run concurrently (tracked per plan ID).
//   - The same plan must not run concurrently. If a trigger (manual or
//     scheduled) fires while a job for that plan is already running, the new
//     trigger is aborted immediately and reported to the server with status
//     "aborted" so it is visible in job history.
type Scheduler struct {
	cron     *cron.Cron
	executor *executor.JobOrchestrator
	reporter ReportFunc
	mu       sync.Mutex
	entryIDs map[string]cron.EntryID // plan_id -> entry

	ctx    context.Context
	cancel context.CancelFunc

	jobMu       sync.RWMutex
	currentJobs map[string]*grpcclient.JobStatus // plan_id -> running job
}

// New creates a new Scheduler.
func New(ctx context.Context, exec *executor.JobOrchestrator, reportFn ReportFunc) *Scheduler {
	ctx, cancel := context.WithCancel(ctx)
	return &Scheduler{
		cron:        cron.New(cron.WithLocation(time.UTC)),
		executor:    exec,
		reporter:    reportFn,
		entryIDs:    make(map[string]cron.EntryID),
		ctx:         ctx,
		cancel:      cancel,
		currentJobs: make(map[string]*grpcclient.JobStatus),
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
			s.runJob(p, r, dr, "scheduled")
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
// If a job for the same plan is already running, the trigger is aborted and a
// JobReport with status "aborted" is emitted so the rejection is visible in
// job history.
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
		s.runJob(targetPlan, repos, defaultRetention, "manual")
	}()
}

// runJob enforces the per-plan concurrency policy and executes the backup job.
// If another job for the same plan is already running, it emits an aborted
// JobReport and returns without starting a new one.
func (s *Scheduler) runJob(
	plan *backupv1.BackupPlan,
	repos []*backupv1.Repository,
	defaultRetention *backupv1.RetentionPolicy,
	trigger string,
) {
	if !s.tryStartJob(plan.GetId(), plan.GetName()) {
		slog.Warn("aborting trigger: plan already running",
			"source", "scheduler",
			"plan", plan.GetName(),
			"plan_id", plan.GetId(),
			"trigger", trigger,
		)
		if s.reporter != nil {
			s.reporter(abortedReport(plan, trigger, "plan already running"))
		}
		return
	}
	defer s.clearCurrentJob(plan.GetId())

	report := s.executor.ExecuteBackupJob(s.ctx, plan, repos, defaultRetention, trigger)
	if s.reporter != nil {
		s.reporter(report)
	}
}

// tryStartJob reserves a per-plan running slot. It returns false if a job for
// the plan is already running, in which case the caller must not start a new
// one.
func (s *Scheduler) tryStartJob(planID, planName string) bool {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if _, exists := s.currentJobs[planID]; exists {
		return false
	}
	s.currentJobs[planID] = &grpcclient.JobStatus{
		PlanName:        planName,
		StartedAt:       time.Now(),
		ProgressPercent: -1, // unknown
	}
	return true
}

// setCurrentJob unconditionally records a running job for the given plan.
// Exposed for tests; production code should use tryStartJob to enforce the
// one-job-per-plan policy.
func (s *Scheduler) setCurrentJob(planID, planName string) {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	s.currentJobs[planID] = &grpcclient.JobStatus{
		PlanName:        planName,
		StartedAt:       time.Now(),
		ProgressPercent: -1, // unknown
	}
}

func (s *Scheduler) clearCurrentJob(planID string) {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	delete(s.currentJobs, planID)
}

// JobStatusFunc returns a function compatible with grpcclient.JobStatusFunc
// that reports a representative currently running job (the one that started
// first), or nil if the agent is idle.
//
// The heartbeat proto currently models a single running job; when multiple
// plans are running concurrently, the earliest-started one is reported.
func (s *Scheduler) JobStatusFunc() grpcclient.JobStatusFunc {
	return func() *grpcclient.JobStatus {
		s.jobMu.RLock()
		defer s.jobMu.RUnlock()
		if len(s.currentJobs) == 0 {
			return nil
		}
		var earliest *grpcclient.JobStatus
		for _, js := range s.currentJobs {
			if earliest == nil || js.StartedAt.Before(earliest.StartedAt) {
				earliest = js
			}
		}
		return earliest
	}
}

// abortedReport builds a minimal JobReport recording that a trigger was
// rejected because the plan was already running.
func abortedReport(plan *backupv1.BackupPlan, trigger, reason string) *backupv1.JobReport {
	now := time.Now()
	return &backupv1.JobReport{
		JobId:      uuid.New().String(),
		PlanId:     plan.GetId(),
		PlanName:   plan.GetName(),
		Type:       "backup",
		Trigger:    trigger,
		Status:     "aborted",
		StartedAt:  timestamppb.New(now),
		FinishedAt: timestamppb.New(now),
		LogTail:    reason,
		LogEntries: []*backupv1.LogEntry{{
			Timestamp: now.Format(time.RFC3339),
			Level:     "warn",
			Source:    "scheduler",
			Message:   reason,
		}},
	}
}
