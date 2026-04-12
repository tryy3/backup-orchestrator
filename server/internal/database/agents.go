package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Agent represents a registered backup agent.
type Agent struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Hostname        string     `json:"hostname"`
	OS              *string    `json:"os,omitempty"`
	Status          string     `json:"status"`
	APIKey          *string    `json:"-"`
	AgentVersion    *string    `json:"agent_version,omitempty"`
	ResticVersion   *string    `json:"restic_version,omitempty"`
	RcloneVersion   *string    `json:"rclone_version,omitempty"`
	RcloneConfig    *string    `json:"rclone_config,omitempty"`
	LastHeartbeat   *time.Time `json:"last_heartbeat,omitempty"`
	LastJobAt       *time.Time `json:"last_job_at,omitempty"`
	ConfigVersion   int        `json:"config_version"`
	ConfigAppliedAt *time.Time `json:"config_applied_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// CreateAgent inserts a new agent record.
func (db *DB) CreateAgent(ctx context.Context, a *Agent) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now

	_, err := db.ExecContext(ctx, `
		INSERT INTO agents (id, name, hostname, os, status, api_key, agent_version, restic_version, rclone_version,
			rclone_config, last_heartbeat, last_job_at, config_version, config_applied_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Hostname, a.OS, a.Status, a.APIKey, a.AgentVersion, a.ResticVersion, a.RcloneVersion,
		a.RcloneConfig, a.LastHeartbeat, a.LastJobAt, a.ConfigVersion, a.ConfigAppliedAt, a.CreatedAt, a.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	return nil
}

// GetAgent retrieves an agent by ID.
func (db *DB) GetAgent(ctx context.Context, id string) (*Agent, error) {
	a := &Agent{}
	err := db.QueryRowContext(ctx, `
		SELECT id, name, hostname, os, status, api_key, agent_version, restic_version, rclone_version,
			rclone_config, last_heartbeat, last_job_at, config_version, config_applied_at, created_at, updated_at
		FROM agents WHERE id = ?`, id,
	).Scan(&a.ID, &a.Name, &a.Hostname, &a.OS, &a.Status, &a.APIKey, &a.AgentVersion, &a.ResticVersion,
		&a.RcloneVersion, &a.RcloneConfig, &a.LastHeartbeat, &a.LastJobAt, &a.ConfigVersion, &a.ConfigAppliedAt,
		&a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	return a, nil
}

// GetAgentByAPIKey retrieves an agent by its API key.
func (db *DB) GetAgentByAPIKey(ctx context.Context, apiKey string) (*Agent, error) {
	a := &Agent{}
	err := db.QueryRowContext(ctx, `
		SELECT id, name, hostname, os, status, api_key, agent_version, restic_version, rclone_version,
			rclone_config, last_heartbeat, last_job_at, config_version, config_applied_at, created_at, updated_at
		FROM agents WHERE api_key = ?`, apiKey,
	).Scan(&a.ID, &a.Name, &a.Hostname, &a.OS, &a.Status, &a.APIKey, &a.AgentVersion, &a.ResticVersion,
		&a.RcloneVersion, &a.RcloneConfig, &a.LastHeartbeat, &a.LastJobAt, &a.ConfigVersion, &a.ConfigAppliedAt,
		&a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get agent by api key: %w", err)
	}
	return a, nil
}

// ListAgents returns all agents.
func (db *DB) ListAgents(ctx context.Context) ([]Agent, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, hostname, os, status, api_key, agent_version, restic_version, rclone_version,
			rclone_config, last_heartbeat, last_job_at, config_version, config_applied_at, created_at, updated_at
		FROM agents ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var agents []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.Hostname, &a.OS, &a.Status, &a.APIKey, &a.AgentVersion,
			&a.ResticVersion, &a.RcloneVersion, &a.RcloneConfig, &a.LastHeartbeat, &a.LastJobAt,
			&a.ConfigVersion, &a.ConfigAppliedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// ApproveAgent sets an agent's status to approved and stores the API key.
func (db *DB) ApproveAgent(ctx context.Context, id, apiKey string) error {
	now := time.Now().UTC()
	result, err := db.ExecContext(ctx, `
		UPDATE agents SET status='approved', api_key=?, updated_at=? WHERE id=? AND status='pending'`,
		apiKey, now, id,
	)
	if err != nil {
		return fmt.Errorf("approve agent: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("approve agent: not found or not pending")
	}
	return nil
}

// RejectAgent sets an agent's status to rejected.
func (db *DB) RejectAgent(ctx context.Context, id string) error {
	now := time.Now().UTC()
	result, err := db.ExecContext(ctx, `
		UPDATE agents SET status='rejected', updated_at=? WHERE id=? AND status='pending'`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("reject agent: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("reject agent: not found or not pending")
	}
	return nil
}

// DeleteAgent removes an agent by ID.
func (db *DB) DeleteAgent(ctx context.Context, id string) error {
	result, err := db.ExecContext(ctx, "DELETE FROM agents WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("delete agent: not found")
	}
	return nil
}

// UpdateHeartbeat updates the agent's last heartbeat time and version info.
func (db *DB) UpdateHeartbeat(ctx context.Context, id, agentVersion, resticVersion, rcloneVersion string) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE agents SET last_heartbeat=?, agent_version=?, restic_version=?, rclone_version=?, updated_at=?
		WHERE id=?`,
		now, agentVersion, resticVersion, rcloneVersion, now, id,
	)
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}
	return nil
}

// UpdateRcloneConfig updates the agent's rclone configuration.
func (db *DB) UpdateRcloneConfig(ctx context.Context, id, config string) error {
	now := time.Now().UTC()
	result, err := db.ExecContext(ctx, `UPDATE agents SET rclone_config=?, updated_at=? WHERE id=?`, config, now, id)
	if err != nil {
		return fmt.Errorf("update rclone config: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update rclone config: agent not found")
	}
	return nil
}

// UpdateConfigVersion atomically increments the agent's config version and returns the new value.
func (db *DB) UpdateConfigVersion(ctx context.Context, id string) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `UPDATE agents SET config_version = config_version + 1, updated_at=? WHERE id=?`, now, id)
	if err != nil {
		return 0, fmt.Errorf("update config version: %w", err)
	}

	var version int
	err = tx.QueryRowContext(ctx, `SELECT config_version FROM agents WHERE id=?`, id).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("read config version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit config version: %w", err)
	}
	return version, nil
}

// UpdateConfigApplied records when an agent last confirmed applying its config.
func (db *DB) UpdateConfigApplied(ctx context.Context, id string, appliedAt time.Time) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `UPDATE agents SET config_applied_at=?, updated_at=? WHERE id=?`, appliedAt, now, id)
	if err != nil {
		return fmt.Errorf("update config applied: %w", err)
	}
	return nil
}
