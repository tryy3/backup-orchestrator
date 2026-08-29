package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

func LoadResolved(ctx context.Context, db *database.DB) (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(registry))
	for _, key := range Keys() {
		def, _ := DefaultJSON(key)
		val, err := db.GetSetting(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("get setting %q: %w", key, err)
		}
		if val == nil {
			out[key] = def
			continue
		}
		raw := json.RawMessage(*val)
		if err := registry[key].validate(raw); err != nil {
			slog.Warn("invalid stored setting; using default", "key", key, "error", err)
			out[key] = def
			continue
		}
		out[key] = raw
	}
	return out, nil
}

func Apply(ctx context.Context, db *database.DB, input map[string]json.RawMessage) error {
	for key, raw := range input {
		if err := db.SetSetting(ctx, key, string(raw)); err != nil {
			return err
		}
	}
	return nil
}
