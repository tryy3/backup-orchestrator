package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PlanHook represents an ordered hook attached to a backup plan.
type PlanHook struct {
	ID        string    `json:"id"`
	PlanID    string    `json:"backup_plan_id"`
	OnEvent   string    `json:"on_event"`
	SortOrder int       `json:"sort_order"`
	ScriptID  *string   `json:"script_id,omitempty"`
	Type      *string   `json:"type,omitempty"`
	Command   *string   `json:"command,omitempty"`
	Timeout   *int      `json:"timeout,omitempty"`
	OnError   *string   `json:"on_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateHook inserts a new plan hook.
func (db *DB) CreateHook(ctx context.Context, h *PlanHook) error {
	h.ID = uuid.New().String()
	now := time.Now().UTC()
	h.CreatedAt = now
	h.UpdatedAt = now

	_, err := db.ExecContext(ctx, `
		INSERT INTO plan_hooks (id, backup_plan_id, on_event, sort_order, script_id, type, command, timeout, on_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.ID, h.PlanID, h.OnEvent, h.SortOrder, h.ScriptID, h.Type, h.Command, h.Timeout, h.OnError, h.CreatedAt, h.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create hook: %w", err)
	}
	return nil
}

// GetHook retrieves a single hook by ID.
func (db *DB) GetHook(ctx context.Context, id string) (*PlanHook, error) {
	h := &PlanHook{}
	err := db.QueryRowContext(ctx, `
		SELECT id, backup_plan_id, on_event, sort_order, script_id, type, command, timeout, on_error, created_at, updated_at
		FROM plan_hooks WHERE id = ?`, id,
	).Scan(&h.ID, &h.PlanID, &h.OnEvent, &h.SortOrder, &h.ScriptID, &h.Type, &h.Command, &h.Timeout, &h.OnError, &h.CreatedAt, &h.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get hook: %w", err)
	}
	return h, nil
}

// ListHooks returns all hooks for a plan, ordered by sort_order.
func (db *DB) ListHooks(ctx context.Context, planID string) ([]PlanHook, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, backup_plan_id, on_event, sort_order, script_id, type, command, timeout, on_error, created_at, updated_at
		FROM plan_hooks WHERE backup_plan_id = ? ORDER BY sort_order, created_at`, planID)
	if err != nil {
		return nil, fmt.Errorf("list hooks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var hooks []PlanHook
	for rows.Next() {
		var h PlanHook
		if err := rows.Scan(&h.ID, &h.PlanID, &h.OnEvent, &h.SortOrder, &h.ScriptID, &h.Type, &h.Command, &h.Timeout, &h.OnError, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan hook: %w", err)
		}
		hooks = append(hooks, h)
	}
	return hooks, rows.Err()
}

// UpdateHook updates an existing plan hook.
func (db *DB) UpdateHook(ctx context.Context, h *PlanHook) error {
	h.UpdatedAt = time.Now().UTC()
	result, err := db.ExecContext(ctx, `
		UPDATE plan_hooks SET on_event=?, sort_order=?, script_id=?, type=?, command=?, timeout=?, on_error=?, updated_at=?
		WHERE id=?`,
		h.OnEvent, h.SortOrder, h.ScriptID, h.Type, h.Command, h.Timeout, h.OnError, h.UpdatedAt, h.ID,
	)
	if err != nil {
		return fmt.Errorf("update hook: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update hook: not found")
	}
	return nil
}

// DeleteHook deletes a plan hook by ID.
func (db *DB) DeleteHook(ctx context.Context, id string) error {
	result, err := db.ExecContext(ctx, "DELETE FROM plan_hooks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete hook: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("delete hook: not found")
	}
	return nil
}

// ReorderHooks updates the sort_order of hooks for a plan based on the provided
// ordered list of hook IDs.
func (db *DB) ReorderHooks(ctx context.Context, planID string, hookIDs []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	for i, hookID := range hookIDs {
		result, err := tx.ExecContext(ctx, `
			UPDATE plan_hooks SET sort_order=?, updated_at=?
			WHERE id=? AND backup_plan_id=?`,
			i, now, hookID, planID,
		)
		if err != nil {
			return fmt.Errorf("reorder hook %s: %w", hookID, err)
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return fmt.Errorf("reorder hooks: hook %s not found for plan %s", hookID, planID)
		}
	}

	return tx.Commit()
}
