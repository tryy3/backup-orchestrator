package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RetentionPolicy defines how many snapshots to keep at each interval.
type RetentionPolicy struct {
	KeepLast    int `json:"keep_last"`
	KeepHourly  int `json:"keep_hourly"`
	KeepDaily   int `json:"keep_daily"`
	KeepWeekly  int `json:"keep_weekly"`
	KeepMonthly int `json:"keep_monthly"`
	KeepYearly  int `json:"keep_yearly"`
}

// BackupPlan defines what to back up, where, when, and how.
type BackupPlan struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	AgentID           string           `json:"agent_id"`
	Paths             []string         `json:"paths"`
	Excludes          []string         `json:"excludes,omitempty"`
	Tags              []string         `json:"tags,omitempty"`
	Schedule          string           `json:"schedule"`
	ForgetAfterBackup bool             `json:"forget_after_backup"`
	PruneAfterForget  bool             `json:"prune_after_forget"`
	PruneSchedule     *string          `json:"prune_schedule,omitempty"`
	Retention         *RetentionPolicy `json:"retention,omitempty"`
	Enabled           bool             `json:"enabled"`
	RepositoryIDs     []string         `json:"repository_ids"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

// CreatePlan inserts a new backup plan and its repository associations.
func (db *DB) CreatePlan(ctx context.Context, p *BackupPlan) error {
	p.ID = uuid.New().String()
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	pathsJSON, err := json.Marshal(p.Paths)
	if err != nil {
		return fmt.Errorf("marshal paths: %w", err)
	}

	var excludesJSON, tagsJSON, retentionJSON *string
	if len(p.Excludes) > 0 {
		b, _ := json.Marshal(p.Excludes)
		s := string(b)
		excludesJSON = &s
	}
	if len(p.Tags) > 0 {
		b, _ := json.Marshal(p.Tags)
		s := string(b)
		tagsJSON = &s
	}
	if p.Retention != nil {
		b, _ := json.Marshal(p.Retention)
		s := string(b)
		retentionJSON = &s
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO backup_plans (id, name, agent_id, paths, excludes, tags, schedule,
			forget_after_backup, prune_after_forget, prune_schedule, retention, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.AgentID, string(pathsJSON), excludesJSON, tagsJSON, p.Schedule,
		p.ForgetAfterBackup, p.PruneAfterForget, p.PruneSchedule, retentionJSON, p.Enabled, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert plan: %w", err)
	}

	for _, repoID := range p.RepositoryIDs {
		_, err = tx.ExecContext(ctx, `INSERT INTO backup_plan_repositories (backup_plan_id, repository_id) VALUES (?, ?)`,
			p.ID, repoID)
		if err != nil {
			return fmt.Errorf("insert plan repository %s: %w", repoID, err)
		}
	}

	return tx.Commit()
}

// GetPlan retrieves a backup plan by ID, including its repository IDs.
func (db *DB) GetPlan(ctx context.Context, id string) (*BackupPlan, error) {
	p := &BackupPlan{}
	var pathsJSON string
	var excludesJSON, tagsJSON, retentionJSON *string

	err := db.QueryRowContext(ctx, `
		SELECT id, name, agent_id, paths, excludes, tags, schedule,
			forget_after_backup, prune_after_forget, prune_schedule, retention, enabled, created_at, updated_at
		FROM backup_plans WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.AgentID, &pathsJSON, &excludesJSON, &tagsJSON, &p.Schedule,
		&p.ForgetAfterBackup, &p.PruneAfterForget, &p.PruneSchedule, &retentionJSON, &p.Enabled,
		&p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get plan: %w", err)
	}

	if err := json.Unmarshal([]byte(pathsJSON), &p.Paths); err != nil {
		return nil, fmt.Errorf("unmarshal paths: %w", err)
	}
	if excludesJSON != nil {
		if err := json.Unmarshal([]byte(*excludesJSON), &p.Excludes); err != nil {
			return nil, fmt.Errorf("unmarshal excludes: %w", err)
		}
	}
	if tagsJSON != nil {
		if err := json.Unmarshal([]byte(*tagsJSON), &p.Tags); err != nil {
			return nil, fmt.Errorf("unmarshal tags: %w", err)
		}
	}
	if retentionJSON != nil {
		p.Retention = &RetentionPolicy{}
		if err := json.Unmarshal([]byte(*retentionJSON), p.Retention); err != nil {
			return nil, fmt.Errorf("unmarshal retention: %w", err)
		}
	}

	// Load repository IDs
	rows, err := db.QueryContext(ctx, "SELECT repository_id FROM backup_plan_repositories WHERE backup_plan_id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("load plan repositories: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var repoID string
		if err := rows.Scan(&repoID); err != nil {
			return nil, fmt.Errorf("scan plan repository: %w", err)
		}
		p.RepositoryIDs = append(p.RepositoryIDs, repoID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plan repositories: %w", err)
	}

	return p, nil
}

// ListPlans returns backup plans optionally filtered by agent ID.
func (db *DB) ListPlans(ctx context.Context, agentID string) ([]BackupPlan, error) {
	query := `SELECT id, name, agent_id, paths, excludes, tags, schedule,
		forget_after_backup, prune_after_forget, prune_schedule, retention, enabled, created_at, updated_at
		FROM backup_plans`
	args := []interface{}{}

	if agentID != "" {
		query += " WHERE agent_id = ?"
		args = append(args, agentID)
	}
	query += " ORDER BY name"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()

	var plans []BackupPlan
	for rows.Next() {
		var p BackupPlan
		var pathsJSON string
		var excludesJSON, tagsJSON, retentionJSON *string

		if err := rows.Scan(&p.ID, &p.Name, &p.AgentID, &pathsJSON, &excludesJSON, &tagsJSON, &p.Schedule,
			&p.ForgetAfterBackup, &p.PruneAfterForget, &p.PruneSchedule, &retentionJSON, &p.Enabled,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}

		if err := json.Unmarshal([]byte(pathsJSON), &p.Paths); err != nil {
			return nil, fmt.Errorf("unmarshal paths: %w", err)
		}
		if excludesJSON != nil {
			if err := json.Unmarshal([]byte(*excludesJSON), &p.Excludes); err != nil {
				return nil, fmt.Errorf("unmarshal excludes for plan %s: %w", p.ID, err)
			}
		}
		if tagsJSON != nil {
			if err := json.Unmarshal([]byte(*tagsJSON), &p.Tags); err != nil {
				return nil, fmt.Errorf("unmarshal tags for plan %s: %w", p.ID, err)
			}
		}
		if retentionJSON != nil {
			p.Retention = &RetentionPolicy{}
			if err := json.Unmarshal([]byte(*retentionJSON), p.Retention); err != nil {
				return nil, fmt.Errorf("unmarshal retention for plan %s: %w", p.ID, err)
			}
		}

		plans = append(plans, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plans: %w", err)
	}

	// Batch-load repository IDs for all plans in a single query.
	if len(plans) > 0 {
		planIndex := make(map[string]int, len(plans))
		placeholders := make([]string, len(plans))
		args := make([]interface{}, len(plans))
		for i, p := range plans {
			planIndex[p.ID] = i
			placeholders[i] = "?"
			args[i] = p.ID
		}

		repoQuery := fmt.Sprintf(
			"SELECT backup_plan_id, repository_id FROM backup_plan_repositories WHERE backup_plan_id IN (%s)",
			strings.Join(placeholders, ","),
		)
		repoRows, err := db.QueryContext(ctx, repoQuery, args...)
		if err != nil {
			return nil, fmt.Errorf("load plan repositories: %w", err)
		}
		defer repoRows.Close()

		for repoRows.Next() {
			var planID, repoID string
			if err := repoRows.Scan(&planID, &repoID); err != nil {
				return nil, fmt.Errorf("scan plan repository: %w", err)
			}
			if idx, ok := planIndex[planID]; ok {
				plans[idx].RepositoryIDs = append(plans[idx].RepositoryIDs, repoID)
			}
		}
		if err := repoRows.Err(); err != nil {
			return nil, fmt.Errorf("iterate plan repositories: %w", err)
		}
	}

	return plans, nil
}

// UpdatePlan updates an existing backup plan and replaces its repository associations.
func (db *DB) UpdatePlan(ctx context.Context, p *BackupPlan) error {
	p.UpdatedAt = time.Now().UTC()

	pathsJSON, err := json.Marshal(p.Paths)
	if err != nil {
		return fmt.Errorf("marshal paths: %w", err)
	}

	var excludesJSON, tagsJSON, retentionJSON *string
	if len(p.Excludes) > 0 {
		b, _ := json.Marshal(p.Excludes)
		s := string(b)
		excludesJSON = &s
	}
	if len(p.Tags) > 0 {
		b, _ := json.Marshal(p.Tags)
		s := string(b)
		tagsJSON = &s
	}
	if p.Retention != nil {
		b, _ := json.Marshal(p.Retention)
		s := string(b)
		retentionJSON = &s
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE backup_plans SET name=?, agent_id=?, paths=?, excludes=?, tags=?, schedule=?,
			forget_after_backup=?, prune_after_forget=?, prune_schedule=?, retention=?, enabled=?, updated_at=?
		WHERE id=?`,
		p.Name, p.AgentID, string(pathsJSON), excludesJSON, tagsJSON, p.Schedule,
		p.ForgetAfterBackup, p.PruneAfterForget, p.PruneSchedule, retentionJSON, p.Enabled, p.UpdatedAt, p.ID,
	)
	if err != nil {
		return fmt.Errorf("update plan: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update plan: not found")
	}

	// Replace repository associations
	_, err = tx.ExecContext(ctx, "DELETE FROM backup_plan_repositories WHERE backup_plan_id = ?", p.ID)
	if err != nil {
		return fmt.Errorf("delete plan repositories: %w", err)
	}
	for _, repoID := range p.RepositoryIDs {
		_, err = tx.ExecContext(ctx, "INSERT INTO backup_plan_repositories (backup_plan_id, repository_id) VALUES (?, ?)",
			p.ID, repoID)
		if err != nil {
			return fmt.Errorf("insert plan repository %s: %w", repoID, err)
		}
	}

	return tx.Commit()
}

// DeletePlan deletes a backup plan by ID.
func (db *DB) DeletePlan(ctx context.Context, id string) error {
	result, err := db.ExecContext(ctx, "DELETE FROM backup_plans WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete plan: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("delete plan: not found")
	}
	return nil
}
