package executor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
	"github.com/tryy3/backup-orchestrator/agent/internal/logging"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// JobOrchestrator coordinates the full lifecycle of a backup job.
type JobOrchestrator struct {
	Restic    *ResticExecutor
	AgentName string
}

// ExecuteBackupJob implements the multi-repo backup flow:
// 1. Generate job ID, record start time
// 2. Build automatic tags
// 3. Run pre_backup hooks (abort if needed)
// 4. Backup to each repository
// 5. Run post_backup hooks (always)
// 6. Determine aggregate status
// 7. Run on_success or on_failure hooks
// 8. Build and return JobReport
func (o *JobOrchestrator) ExecuteBackupJob(
	ctx context.Context,
	plan *backupv1.BackupPlan,
	repos []*backupv1.Repository,
	defaultRetention *backupv1.RetentionPolicy,
	trigger string,
) *backupv1.JobReport {
	jobID := uuid.New().String()
	startedAt := time.Now()

	// Create per-job logger: writes to both console and buffer.
	buf := logging.NewBufferHandler(slog.LevelInfo)
	multi := logging.NewMultiHandler(slog.Default().Handler(), buf)
	jlog := slog.New(multi)

	jlog.Info("starting backup job",
		"source", "orchestrator",
		"job_id", jobID,
		"plan", plan.GetName(),
		"trigger", trigger,
	)

	report := &backupv1.JobReport{
		JobId:     jobID,
		PlanId:    plan.GetId(),
		PlanName:  plan.GetName(),
		Type:      "backup",
		Trigger:   trigger,
		StartedAt: timestamppb.New(startedAt),
	}

	// Build automatic tags.
	tags := []string{
		fmt.Sprintf("agent:%s", o.AgentName),
		fmt.Sprintf("plan:%s", plan.GetName()),
		fmt.Sprintf("trigger:%s", trigger),
	}
	tags = append(tags, plan.GetTags()...)

	// Build hook context (pre-backup only has PlanName and Hostname).
	hctx := &HookContext{
		PlanName:  plan.GetName(),
		Hostname:  o.AgentName,
		StartedAt: startedAt.Format(time.RFC3339),
	}

	var allHookResults []*HookResult

	// Step 3: Run pre_backup hooks.
	preResults, aborted := RunHooks(ctx, plan.GetHooks(), "pre_backup", hctx, jlog)
	allHookResults = append(allHookResults, preResults...)

	if aborted {
		jlog.Warn("pre_backup hooks aborted, skipping backup", "source", "orchestrator")
		report.Status = "failed"
		finishedAt := time.Now()
		report.FinishedAt = timestamppb.New(finishedAt)
		hctx.Status = "failed"
		hctx.FinishedAt = finishedAt.Format(time.RFC3339)
		hctx.Error = "pre_backup hook aborted"

		// Run on_failure hooks.
		failResults, _ := RunHooks(ctx, plan.GetHooks(), "on_failure", hctx, jlog)
		allHookResults = append(allHookResults, failResults...)

		report.HookResults = convertHookResults(allHookResults)
		report.LogEntries = buf.Entries()
		report.LogTail = buf.PlainText()
		return report
	}

	// Step 4: Build repo lookup and run backup for each targeted repository.
	repoMap := make(map[string]*backupv1.Repository)
	for _, r := range repos {
		repoMap[r.GetId()] = r
	}

	var repoResults []*backupv1.RepositoryResult
	successCount := 0
	failCount := 0

	for _, repoID := range plan.GetRepositoryIds() {
		pbRepo, ok := repoMap[repoID]
		if !ok {
			jlog.Warn("repository not found, skipping",
				"source", "orchestrator",
				"repository_id", repoID,
			)
			repoResults = append(repoResults, &backupv1.RepositoryResult{
				RepositoryId:   repoID,
				RepositoryName: "unknown",
				Status:         "skipped",
				Error:          "repository not found in config",
			})
			failCount++
			continue
		}

		repo := Repository{
			ID:       pbRepo.GetId(),
			Name:     pbRepo.GetName(),
			Type:     pbRepo.GetType(),
			Path:     pbRepo.GetPath(),
			Password: pbRepo.GetPassword(),
		}

		jlog.Info("backing up to repository",
			"source", "orchestrator",
			"repository", repo.Name,
			"path", repo.Path,
		)

		result := o.backupToRepo(ctx, repo, plan, tags, defaultRetention, jlog)
		result.RepositoryId = repoID
		result.RepositoryName = pbRepo.GetName()
		repoResults = append(repoResults, result)

		if result.Status == "success" {
			successCount++
		} else {
			failCount++
		}
	}

	report.RepositoryResults = repoResults

	// Step 5: Run post_backup hooks (always).
	finishedAt := time.Now()
	duration := finishedAt.Sub(startedAt)

	// Determine aggregate status.
	var aggregateStatus string
	switch {
	case failCount == 0:
		aggregateStatus = "success"
	case successCount == 0:
		aggregateStatus = "failed"
	default:
		aggregateStatus = "partial"
	}

	// Update hook context with results.
	hctx.Status = aggregateStatus
	hctx.Duration = duration.String()
	hctx.FinishedAt = finishedAt.Format(time.RFC3339)

	// Collect aggregate stats from successful repos for hook context.
	var totalBytesAdded, totalFilesNew, totalFilesChanged int64
	var lastSnapshotID string
	for _, rr := range repoResults {
		if rr.Status == "success" {
			totalBytesAdded += rr.BytesAdded
			totalFilesNew += rr.FilesNew
			totalFilesChanged += rr.FilesChanged
			if rr.SnapshotId != "" {
				lastSnapshotID = rr.SnapshotId
			}
		}
	}
	hctx.BytesAdded = fmt.Sprintf("%d", totalBytesAdded)
	hctx.FilesNew = fmt.Sprintf("%d", totalFilesNew)
	hctx.FilesChanged = fmt.Sprintf("%d", totalFilesChanged)
	hctx.SnapshotID = lastSnapshotID

	if aggregateStatus == "failed" {
		hctx.Error = "all repositories failed"
	}

	postResults, _ := RunHooks(ctx, plan.GetHooks(), "post_backup", hctx, jlog)
	allHookResults = append(allHookResults, postResults...)

	// Step 7: Run on_success or on_failure hooks.
	if aggregateStatus == "success" {
		successResults, _ := RunHooks(ctx, plan.GetHooks(), "on_success", hctx, jlog)
		allHookResults = append(allHookResults, successResults...)
	} else {
		failResults, _ := RunHooks(ctx, plan.GetHooks(), "on_failure", hctx, jlog)
		allHookResults = append(allHookResults, failResults...)
	}

	report.Status = aggregateStatus
	report.FinishedAt = timestamppb.New(finishedAt)
	report.HookResults = convertHookResults(allHookResults)
	report.LogEntries = buf.Entries()
	report.LogTail = buf.PlainText()

	jlog.Info("backup job completed",
		"source", "orchestrator",
		"job_id", jobID,
		"status", aggregateStatus,
		"duration", duration.String(),
	)
	return report
}

// backupToRepo runs the backup+forget+prune cycle for a single repository.
func (o *JobOrchestrator) backupToRepo(
	ctx context.Context,
	repo Repository,
	plan *backupv1.BackupPlan,
	tags []string,
	defaultRetention *backupv1.RetentionPolicy,
	jlog *slog.Logger,
) *backupv1.RepositoryResult {
	result := &backupv1.RepositoryResult{
		Status: "success",
	}

	repoStart := time.Now()

	// Ensure repo is initialized.
	jlog.Info("ensuring repository is initialized", "source", "restic", "repository", repo.Name)
	if err := o.Restic.EnsureRepo(ctx, repo); err != nil {
		jlog.Error("ensure repo failed", "source", "restic", "repository", repo.Name, "error", err)
		result.Status = "failed"
		result.Error = fmt.Sprintf("ensure repo: %v", err)
		result.DurationMs = time.Since(repoStart).Milliseconds()
		return result
	}

	// Run backup.
	jlog.Info("running restic backup", "source", "restic", "repository", repo.Name)
	backupResult, err := o.Restic.Backup(ctx, repo, plan.GetPaths(), plan.GetExcludes(), tags)
	if err != nil {
		jlog.Error("backup failed", "source", "restic", "repository", repo.Name, "error", err)
		result.Status = "failed"
		result.Error = fmt.Sprintf("backup: %v", err)
		result.DurationMs = time.Since(repoStart).Milliseconds()
		return result
	}

	// Log restic stderr if present (may contain warnings).
	if backupResult.Stderr != "" {
		jlog.Warn("restic stderr output", "source", "restic", "repository", repo.Name, "stderr", backupResult.Stderr)
	}

	jlog.Info("backup succeeded",
		"source", "restic",
		"repository", repo.Name,
		"snapshot_id", backupResult.SnapshotID,
		"files_new", backupResult.FilesNew,
		"files_changed", backupResult.FilesChanged,
		"bytes_added", backupResult.BytesAdded,
	)

	result.SnapshotId = backupResult.SnapshotID
	result.FilesNew = backupResult.FilesNew
	result.FilesChanged = backupResult.FilesChanged
	result.FilesUnmodified = backupResult.FilesUnmodified
	result.BytesAdded = backupResult.BytesAdded
	result.TotalBytes = backupResult.TotalBytes

	// Run forget if configured.
	if plan.GetForgetAfterBackup() {
		retention := resolveRetention(plan.GetRetention(), defaultRetention)
		if retention != nil {
			rp := RetentionPolicy{
				KeepLast:    retention.GetKeepLast(),
				KeepHourly:  retention.GetKeepHourly(),
				KeepDaily:   retention.GetKeepDaily(),
				KeepWeekly:  retention.GetKeepWeekly(),
				KeepMonthly: retention.GetKeepMonthly(),
				KeepYearly:  retention.GetKeepYearly(),
			}
			jlog.Info("running forget", "source", "restic", "repository", repo.Name)
			if err := o.Restic.Forget(ctx, repo, rp, tags); err != nil {
				jlog.Warn("forget failed", "source", "restic", "repository", repo.Name, "error", err)
				// Forget failure doesn't fail the backup.
			} else {
				jlog.Info("forget completed", "source", "restic", "repository", repo.Name)
			}

			// Run prune if configured.
			if plan.GetPruneAfterForget() {
				jlog.Info("running prune", "source", "restic", "repository", repo.Name)
				if err := o.Restic.Prune(ctx, repo); err != nil {
					jlog.Warn("prune failed", "source", "restic", "repository", repo.Name, "error", err)
					// Prune failure doesn't fail the backup.
				} else {
					jlog.Info("prune completed", "source", "restic", "repository", repo.Name)
				}
			}
		}
	}

	result.DurationMs = time.Since(repoStart).Milliseconds()
	return result
}

// resolveRetention returns the plan-level retention if set, otherwise the default.
func resolveRetention(planRetention, defaultRetention *backupv1.RetentionPolicy) *backupv1.RetentionPolicy {
	if planRetention != nil {
		return planRetention
	}
	return defaultRetention
}

// convertHookResults converts internal HookResult to protobuf HookResult.
func convertHookResults(results []*HookResult) []*backupv1.HookResult {
	var out []*backupv1.HookResult
	for _, r := range results {
		out = append(out, &backupv1.HookResult{
			HookName:   r.HookName,
			Phase:      r.Phase,
			Status:     r.Status,
			Error:      r.Error,
			DurationMs: r.DurationMs,
		})
	}
	return out
}
