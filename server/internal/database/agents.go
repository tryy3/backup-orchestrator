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
	// CommandTimeouts is the per-agent override of the global command timeout
	// settings, stored as JSON. Nil/empty means "no override, use globals".
	CommandTimeouts *string `json:"command_timeouts,omitempty"`
	// OutboxOverrides is the per-agent override of the global outbox tunables,
	// stored as JSON. Nil/empty means "no override, use globals".
	OutboxOverrides *string   `json:"outbox_overrides,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CreateAgent inserts a new agent record.
func (db *DB) CreateAgent(ctx context.Context, a *Agent) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now

	encRclone, err := db.encryptPtr(a.RcloneConfig)
	if err != nil {
		return fmt.Errorf("encrypt rclone config: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO agents (id, name, hostname, os, status, api_key, agent_version, restic_version, rclone_version,
			rclone_config, last_heartbeat, last_job_at, config_version, config_applied_at, command_timeouts, outbox_overrides, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Hostname, a.OS, a.Status, a.APIKey, a.AgentVersion, a.ResticVersion, a.RcloneVersion,
		encRclone, a.LastHeartbeat, a.LastJobAt, a.ConfigVersion, a.ConfigAppliedAt, a.CommandTimeouts, a.OutboxOverrides, a.CreatedAt, a.UpdatedAt,
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
			rclone_config, last_heartbeat, last_job_at, config_version, config_applied_at, command_timeouts, outbox_overrides, created_at, updated_at
		FROM agents WHERE id = ?`, id,
	).Scan(&a.ID, &a.Name, &a.Hostname, &a.OS, &a.Status, &a.APIKey, &a.AgentVersion, &a.ResticVersion,
		&a.RcloneVersion, &a.RcloneConfig, &a.LastHeartbeat, &a.LastJobAt, &a.ConfigVersion, &a.ConfigAppliedAt,
		&a.CommandTimeouts, &a.OutboxOverrides, &a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	a.RcloneConfig, err = db.decryptPtr(a.RcloneConfig)
	if err != nil {
		return nil, fmt.Errorf("decrypt agent rclone config: %w", err)
	}
	return a, nil
}

// GetAgentByAPIKey retrieves an agent by its API key.
func (db *DB) GetAgentByAPIKey(ctx context.Context, apiKey string) (*Agent, error) {
	a := &Agent{}
	err := db.QueryRowContext(ctx, `
		SELECT id, name, hostname, os, status, api_key, agent_version, restic_version, rclone_version,
			rclone_config, last_heartbeat, last_job_at, config_version, config_applied_at, command_timeouts, outbox_overrides, created_at, updated_at
		FROM agents WHERE api_key = ?`, apiKey,
	).Scan(&a.ID, &a.Name, &a.Hostname, &a.OS, &a.Status, &a.APIKey, &a.AgentVersion, &a.ResticVersion,
		&a.RcloneVersion, &a.RcloneConfig, &a.LastHeartbeat, &a.LastJobAt, &a.ConfigVersion, &a.ConfigAppliedAt,
		&a.CommandTimeouts, &a.OutboxOverrides, &a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get agent by api key: %w", err)
	}
	a.RcloneConfig, err = db.decryptPtr(a.RcloneConfig)
	if err != nil {
		return nil, fmt.Errorf("decrypt agent rclone config: %w", err)
	}
	return a, nil
}

// ListAgents returns all agents.
func (db *DB) ListAgents(ctx context.Context) ([]Agent, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, hostname, os, status, api_key, agent_version, restic_version, rclone_version,
			rclone_config, last_heartbeat, last_job_at, config_version, config_applied_at, command_timeouts, outbox_overrides, created_at, updated_at
		FROM agents ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var agents []Agent
	for rows.Next() {
		var a Agent
		if err = rows.Scan(&a.ID, &a.Name, &a.Hostname, &a.OS, &a.Status, &a.APIKey, &a.AgentVersion,
			&a.ResticVersion, &a.RcloneVersion, &a.RcloneConfig, &a.LastHeartbeat, &a.LastJobAt,
			&a.ConfigVersion, &a.ConfigAppliedAt, &a.CommandTimeouts, &a.OutboxOverrides, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		a.RcloneConfig, err = db.decryptPtr(a.RcloneConfig)
		if err != nil {
			return nil, fmt.Errorf("decrypt agent %s rclone config: %w", a.ID, err)
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
	encConfig, err := db.encrypt(config)
	if err != nil {
		return fmt.Errorf("encrypt rclone config: %w", err)
	}
	now := time.Now().UTC()
	result, err := db.ExecContext(ctx, `UPDATE agents SET rclone_config=?, updated_at=? WHERE id=?`, encConfig, now, id)
	if err != nil {
		return fmt.Errorf("update rclone config: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update rclone config: agent not found")
	}
	return nil
}

// UpdateCommandTimeouts stores per-agent command timeout overrides as a JSON
// document, or clears them when timeoutsJSON is nil/empty.
func (db *DB) UpdateCommandTimeouts(ctx context.Context, id string, timeoutsJSON *string) error {
	now := time.Now().UTC()
	result, err := db.ExecContext(ctx,
		`UPDATE agents SET command_timeouts=?, updated_at=? WHERE id=?`,
		timeoutsJSON, now, id)
	if err != nil {
		return fmt.Errorf("update command timeouts: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update command timeouts: agent not found")
	}
	return nil
}

// UpdateOutboxOverrides stores per-agent outbox tunable overrides as a JSON
// document, or clears them when overridesJSON is nil/empty.
func (db *DB) UpdateOutboxOverrides(ctx context.Context, id string, overridesJSON *string) error {
	now := time.Now().UTC()
	result, err := db.ExecContext(ctx,
		`UPDATE agents SET outbox_overrides=?, updated_at=? WHERE id=?`,
		overridesJSON, now, id)
	if err != nil {
		return fmt.Errorf("update outbox overrides: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update outbox overrides: agent not found")
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
