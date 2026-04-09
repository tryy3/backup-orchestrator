package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// LogEntry represents a single structured log entry from a job execution.
type LogEntry struct {
	Timestamp  string            `json:"timestamp"`
	Level      string            `json:"level"`
	Source     string            `json:"source"`
	Message    string            `json:"message"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Job represents a single execution of a backup plan or manual operation.
type Job struct {
	ID                string                `json:"id"`
	AgentID           string                `json:"agent_id"`
	PlanID            *string               `json:"plan_id,omitempty"`
	PlanName          string                `json:"plan_name"`
	Type              string                `json:"type"`
	Trigger           string                `json:"trigger"`
	Status            string                `json:"status"`
	StartedAt         time.Time             `json:"started_at"`
	FinishedAt        *time.Time            `json:"finished_at,omitempty"`
	LogTail           *string               `json:"-"`
	CreatedAt         time.Time             `json:"created_at"`
	RepositoryResults []JobRepositoryResult `json:"repository_results,omitempty"`
	HookResults       []JobHookResult       `json:"hook_results,omitempty"`
	LogEntries        []LogEntry            `json:"log_entries,omitempty"`
}

// JobRepositoryResult tracks the outcome of a backup job for a single repository.
type JobRepositoryResult struct {
	ID              string  `json:"id"`
	JobID           string  `json:"job_id"`
	RepositoryID    string  `json:"repository_id"`
	RepositoryName  string  `json:"repository_name"`
	Status          string  `json:"status"`
	SnapshotID      *string `json:"snapshot_id,omitempty"`
	Error           *string `json:"error,omitempty"`
	FilesNew        *int64  `json:"files_new,omitempty"`
	FilesChanged    *int64  `json:"files_changed,omitempty"`
	FilesUnmodified *int64  `json:"files_unmodified,omitempty"`
	BytesAdded      *int64  `json:"bytes_added,omitempty"`
	TotalBytes      *int64  `json:"total_bytes,omitempty"`
	DurationMs      *int64  `json:"duration_ms,omitempty"`
}

// JobHookResult tracks the outcome of a hook execution within a job.
type JobHookResult struct {
	ID         string  `json:"id"`
	JobID      string  `json:"job_id"`
	HookName   string  `json:"hook_name"`
	Phase      string  `json:"phase"`
	Status     string  `json:"status"`
	Error      *string `json:"error,omitempty"`
	DurationMs *int64  `json:"duration_ms,omitempty"`
}

// CreateJob inserts a job along with its repository results and hook results.
func (db *DB) CreateJob(ctx context.Context, j *Job) error {
	if j.ID == "" {
		j.ID = uuid.New().String()
	}
	j.CreatedAt = time.Now().UTC()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO jobs (id, agent_id, plan_id, plan_name, type, trigger, status, started_at, finished_at, log_tail, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.ID, j.AgentID, j.PlanID, j.PlanName, j.Type, j.Trigger, j.Status, j.StartedAt, j.FinishedAt, j.LogTail, j.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}

	for _, rr := range j.RepositoryResults {
		if rr.ID == "" {
			rr.ID = uuid.New().String()
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO job_repository_results (id, job_id, repository_id, repository_name, status, snapshot_id, error,
				files_new, files_changed, files_unmodified, bytes_added, total_bytes, duration_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			rr.ID, j.ID, rr.RepositoryID, rr.RepositoryName, rr.Status, rr.SnapshotID, rr.Error,
			rr.FilesNew, rr.FilesChanged, rr.FilesUnmodified, rr.BytesAdded, rr.TotalBytes, rr.DurationMs,
		)
		if err != nil {
			return fmt.Errorf("insert repository result: %w", err)
		}
	}

	for _, hr := range j.HookResults {
		if hr.ID == "" {
			hr.ID = uuid.New().String()
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO job_hook_results (id, job_id, hook_name, phase, status, error, duration_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			hr.ID, j.ID, hr.HookName, hr.Phase, hr.Status, hr.Error, hr.DurationMs,
		)
		if err != nil {
			return fmt.Errorf("insert hook result: %w", err)
		}
	}

	return tx.Commit()
}

// GetJob retrieves a job by ID with its repository and hook results.
func (db *DB) GetJob(ctx context.Context, id string) (*Job, error) {
	j := &Job{}
	err := db.QueryRowContext(ctx, `
		SELECT id, agent_id, plan_id, plan_name, type, trigger, status, started_at, finished_at, log_tail, created_at
		FROM jobs WHERE id = ?`, id,
	).Scan(&j.ID, &j.AgentID, &j.PlanID, &j.PlanName, &j.Type, &j.Trigger, &j.Status, &j.StartedAt, &j.FinishedAt, &j.LogTail, &j.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}

	// Load repository results
	repoRows, err := db.QueryContext(ctx, `
		SELECT id, job_id, repository_id, repository_name, status, snapshot_id, error,
			files_new, files_changed, files_unmodified, bytes_added, total_bytes, duration_ms
		FROM job_repository_results WHERE job_id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("load repository results: %w", err)
	}
	defer repoRows.Close()
	for repoRows.Next() {
		var rr JobRepositoryResult
		if err := repoRows.Scan(&rr.ID, &rr.JobID, &rr.RepositoryID, &rr.RepositoryName, &rr.Status,
			&rr.SnapshotID, &rr.Error, &rr.FilesNew, &rr.FilesChanged, &rr.FilesUnmodified,
			&rr.BytesAdded, &rr.TotalBytes, &rr.DurationMs); err != nil {
			return nil, fmt.Errorf("scan repository result: %w", err)
		}
		j.RepositoryResults = append(j.RepositoryResults, rr)
	}
	if err := repoRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repository results: %w", err)
	}

	// Load hook results
	hookRows, err := db.QueryContext(ctx, `
		SELECT id, job_id, hook_name, phase, status, error, duration_ms
		FROM job_hook_results WHERE job_id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("load hook results: %w", err)
	}
	defer hookRows.Close()
	for hookRows.Next() {
		var hr JobHookResult
		if err := hookRows.Scan(&hr.ID, &hr.JobID, &hr.HookName, &hr.Phase, &hr.Status, &hr.Error, &hr.DurationMs); err != nil {
			return nil, fmt.Errorf("scan hook result: %w", err)
		}
		j.HookResults = append(j.HookResults, hr)
	}
	if err := hookRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hook results: %w", err)
	}

	j.parseLogEntries()
	return j, nil
}

// parseLogEntries deserializes structured log entries from the log_tail JSON column.
func (j *Job) parseLogEntries() {
	if j.LogTail == nil || *j.LogTail == "" {
		return
	}
	var entries []LogEntry
	if err := json.Unmarshal([]byte(*j.LogTail), &entries); err != nil {
		// Fallback: treat as plain text (old format or non-JSON).
		j.LogEntries = []LogEntry{{
			Timestamp: j.StartedAt.Format(time.RFC3339),
			Level:     "info",
			Source:    "agent",
			Message:   *j.LogTail,
		}}
		return
	}
	j.LogEntries = entries
}

// ListJobs returns jobs filtered by agent ID, plan ID, and/or status.
// Pass empty strings to skip a filter. Results are paginated with limit and offset.
func (db *DB) ListJobs(ctx context.Context, agentID, planID, status string, limit, offset int) ([]Job, error) {
	query := `SELECT id, agent_id, plan_id, plan_name, type, trigger, status, started_at, finished_at, log_tail, created_at
		FROM jobs WHERE 1=1`
	args := []interface{}{}

	if agentID != "" {
		query += " AND agent_id = ?"
		args = append(args, agentID)
	}
	if planID != "" {
		query += " AND plan_id = ?"
		args = append(args, planID)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	query += " ORDER BY started_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.AgentID, &j.PlanID, &j.PlanName, &j.Type, &j.Trigger, &j.Status,
			&j.StartedAt, &j.FinishedAt, &j.LogTail, &j.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// UpdateJobStatus updates the status and optional finished_at and log_tail of a job.
func (db *DB) UpdateJobStatus(ctx context.Context, id, status string, finishedAt *time.Time, logTail *string) error {
	result, err := db.ExecContext(ctx, `
		UPDATE jobs SET status=?, finished_at=?, log_tail=? WHERE id=?`,
		status, finishedAt, logTail, id,
	)
	if err != nil {
		return fmt.Errorf("update job status: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update job status: not found")
	}
	return nil
}

// CreatePlannedJob inserts a placeholder job with status "planned" when a backup is triggered.
// The job is updated later when the agent reports it as started/completed.
func (db *DB) CreatePlannedJob(ctx context.Context, agentID, planID, planName, trigger string) (*Job, error) {
	j := &Job{
		ID:        uuid.New().String(),
		AgentID:   agentID,
		PlanID:    &planID,
		PlanName:  planName,
		Type:      "backup",
		Trigger:   trigger,
		Status:    "planned",
		StartedAt: time.Now().UTC(),
		CreatedAt: time.Now().UTC(),
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO jobs (id, agent_id, plan_id, plan_name, type, trigger, status, started_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.ID, j.AgentID, j.PlanID, j.PlanName, j.Type, j.Trigger, j.Status, j.StartedAt, j.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create planned job: %w", err)
	}
	return j, nil
}

// FindPlannedJob finds a planned/running job for a given agent and plan.
func (db *DB) FindPlannedJob(ctx context.Context, agentID, planID string) (*Job, error) {
	j := &Job{}
	err := db.QueryRowContext(ctx, `
		SELECT id, agent_id, plan_id, plan_name, type, trigger, status, started_at, finished_at, log_tail, created_at
		FROM jobs WHERE agent_id = ? AND plan_id = ? AND status IN ('planned', 'running')
		ORDER BY created_at DESC LIMIT 1`,
		agentID, planID,
	).Scan(&j.ID, &j.AgentID, &j.PlanID, &j.PlanName, &j.Type, &j.Trigger, &j.Status, &j.StartedAt, &j.FinishedAt, &j.LogTail, &j.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find planned job: %w", err)
	}
	return j, nil
}

// CompleteJob updates a planned/running job with final results (status, timestamps, logs, results).
func (db *DB) CompleteJob(ctx context.Context, j *Job) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		UPDATE jobs SET status=?, started_at=?, finished_at=?, log_tail=? WHERE id=?`,
		j.Status, j.StartedAt, j.FinishedAt, j.LogTail, j.ID,
	)
	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}

	for _, rr := range j.RepositoryResults {
		if rr.ID == "" {
			rr.ID = uuid.New().String()
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO job_repository_results (id, job_id, repository_id, repository_name, status, snapshot_id, error,
				files_new, files_changed, files_unmodified, bytes_added, total_bytes, duration_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			rr.ID, j.ID, rr.RepositoryID, rr.RepositoryName, rr.Status, rr.SnapshotID, rr.Error,
			rr.FilesNew, rr.FilesChanged, rr.FilesUnmodified, rr.BytesAdded, rr.TotalBytes, rr.DurationMs,
		)
		if err != nil {
			return fmt.Errorf("insert repository result: %w", err)
		}
	}

	for _, hr := range j.HookResults {
		if hr.ID == "" {
			hr.ID = uuid.New().String()
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO job_hook_results (id, job_id, hook_name, phase, status, error, duration_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			hr.ID, j.ID, hr.HookName, hr.Phase, hr.Status, hr.Error, hr.DurationMs,
		)
		if err != nil {
			return fmt.Errorf("insert hook result: %w", err)
		}
	}

	return tx.Commit()
}
