package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Script represents a reusable hook definition.
type Script struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Command   string    `json:"command"`
	Timeout   int       `json:"timeout"`
	OnError   string    `json:"on_error"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateScript inserts a new script.
func (db *DB) CreateScript(s *Script) error {
	s.ID = uuid.New().String()
	now := time.Now().UTC()
	s.CreatedAt = now
	s.UpdatedAt = now

	if s.Type == "" {
		s.Type = "command"
	}
	if s.Timeout == 0 {
		s.Timeout = 60
	}
	if s.OnError == "" {
		s.OnError = "continue"
	}

	_, err := db.Exec(`
		INSERT INTO scripts (id, name, type, command, timeout, on_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.Type, s.Command, s.Timeout, s.OnError, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create script: %w", err)
	}
	return nil
}

// GetScript retrieves a script by ID.
func (db *DB) GetScript(id string) (*Script, error) {
	s := &Script{}
	err := db.QueryRow(`
		SELECT id, name, type, command, timeout, on_error, created_at, updated_at
		FROM scripts WHERE id = ?`, id,
	).Scan(&s.ID, &s.Name, &s.Type, &s.Command, &s.Timeout, &s.OnError, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get script: %w", err)
	}
	return s, nil
}

// ListScripts returns all scripts ordered by name.
func (db *DB) ListScripts() ([]Script, error) {
	rows, err := db.Query(`
		SELECT id, name, type, command, timeout, on_error, created_at, updated_at
		FROM scripts ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list scripts: %w", err)
	}
	defer rows.Close()

	var scripts []Script
	for rows.Next() {
		var s Script
		if err := rows.Scan(&s.ID, &s.Name, &s.Type, &s.Command, &s.Timeout, &s.OnError, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan script: %w", err)
		}
		scripts = append(scripts, s)
	}
	return scripts, rows.Err()
}

// UpdateScript updates an existing script.
func (db *DB) UpdateScript(s *Script) error {
	s.UpdatedAt = time.Now().UTC()
	result, err := db.Exec(`
		UPDATE scripts SET name=?, type=?, command=?, timeout=?, on_error=?, updated_at=?
		WHERE id=?`,
		s.Name, s.Type, s.Command, s.Timeout, s.OnError, s.UpdatedAt, s.ID,
	)
	if err != nil {
		return fmt.Errorf("update script: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update script: not found")
	}
	return nil
}

// AgentIDsUsingScript returns the distinct agent IDs whose plans have hooks referencing the given script.
func (db *DB) AgentIDsUsingScript(scriptID string) ([]string, error) {
	rows, err := db.Query(`
		SELECT DISTINCT bp.agent_id FROM backup_plans bp
		JOIN plan_hooks ph ON ph.backup_plan_id = bp.id
		WHERE ph.script_id = ?`, scriptID)
	if err != nil {
		return nil, fmt.Errorf("agents using script: %w", err)
	}
	defer rows.Close()
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

// DeleteScript deletes a script by ID, returning an error if it is referenced by plan hooks.
func (db *DB) DeleteScript(id string) error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM plan_hooks WHERE script_id = ?", id).Scan(&count)
	if err != nil {
		return fmt.Errorf("check script references: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("script is referenced by %d hook(s)", count)
	}

	result, err := db.Exec("DELETE FROM scripts WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete script: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("delete script: not found")
	}
	return nil
}
