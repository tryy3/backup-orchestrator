package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Repository represents a restic backup repository.
type Repository struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Scope     string    `json:"scope"`
	AgentID   *string   `json:"agent_id,omitempty"`
	Type      string    `json:"type"`
	Path      string    `json:"path"`
	Password  string    `json:"password"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateRepository inserts a new repository and returns the created record.
func (db *DB) CreateRepository(ctx context.Context, r *Repository) error {
	r.ID = uuid.New().String()
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now

	_, err := db.ExecContext(ctx, `
		INSERT INTO repositories (id, name, scope, agent_id, type, path, password, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.Scope, r.AgentID, r.Type, r.Path, r.Password, r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create repository: %w", err)
	}
	return nil
}

// GetRepository retrieves a single repository by ID.
func (db *DB) GetRepository(ctx context.Context, id string) (*Repository, error) {
	r := &Repository{}
	err := db.QueryRowContext(ctx, `
		SELECT id, name, scope, agent_id, type, path, password, created_at, updated_at
		FROM repositories WHERE id = ?`, id,
	).Scan(&r.ID, &r.Name, &r.Scope, &r.AgentID, &r.Type, &r.Path, &r.Password, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get repository: %w", err)
	}
	return r, nil
}

// ListRepositories returns repositories filtered by scope and/or agent ID.
// Pass empty strings to skip a filter.
func (db *DB) ListRepositories(ctx context.Context, scope, agentID string) ([]Repository, error) {
	query := "SELECT id, name, scope, agent_id, type, path, password, created_at, updated_at FROM repositories WHERE 1=1"
	args := []interface{}{}

	if scope != "" {
		query += " AND scope = ?"
		args = append(args, scope)
	}
	if agentID != "" {
		query += " AND agent_id = ?"
		args = append(args, agentID)
	}
	query += " ORDER BY name"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var repos []Repository
	for rows.Next() {
		var r Repository
		if err := rows.Scan(&r.ID, &r.Name, &r.Scope, &r.AgentID, &r.Type, &r.Path, &r.Password, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan repository: %w", err)
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

// UpdateRepository updates an existing repository.
func (db *DB) UpdateRepository(ctx context.Context, r *Repository) error {
	r.UpdatedAt = time.Now().UTC()
	result, err := db.ExecContext(ctx, `
		UPDATE repositories SET name=?, scope=?, agent_id=?, type=?, path=?, password=?, updated_at=?
		WHERE id=?`,
		r.Name, r.Scope, r.AgentID, r.Type, r.Path, r.Password, r.UpdatedAt, r.ID,
	)
	if err != nil {
		return fmt.Errorf("update repository: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update repository: not found")
	}
	return nil
}

// AgentIDsUsingRepository returns the distinct agent IDs whose plans reference the given repository.
func (db *DB) AgentIDsUsingRepository(ctx context.Context, repoID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT bp.agent_id FROM backup_plans bp
		JOIN backup_plan_repositories bpr ON bpr.backup_plan_id = bp.id
		WHERE bpr.repository_id = ?`, repoID)
	if err != nil {
		return nil, fmt.Errorf("agents using repository: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan agent id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteRepository deletes a repository by ID.
func (db *DB) DeleteRepository(ctx context.Context, id string) error {
	result, err := db.ExecContext(ctx, "DELETE FROM repositories WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete repository: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("delete repository: not found")
	}
	return nil
}

// GetRepositoriesByIDs returns repositories matching the given IDs in a single query.
func (db *DB) GetRepositoriesByIDs(ctx context.Context, ids []string) (map[string]*Repository, error) {
	if len(ids) == 0 {
		return make(map[string]*Repository), nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(
		"SELECT id, name, scope, agent_id, type, path, password, created_at, updated_at FROM repositories WHERE id IN (%s)",
		strings.Join(placeholders, ","),
	)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get repositories by ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]*Repository, len(ids))
	for rows.Next() {
		var r Repository
		if err := rows.Scan(&r.ID, &r.Name, &r.Scope, &r.AgentID, &r.Type, &r.Path, &r.Password, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan repository: %w", err)
		}
		result[r.ID] = &r
	}
	return result, rows.Err()
}
