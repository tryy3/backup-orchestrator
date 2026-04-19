package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

// settingsKeys lists all known settings keys that the GET handler will return.
var settingsKeys = []string{
	"default_retention",
	"heartbeat_interval_seconds",
	"agent_offline_threshold_seconds",
	"job_history_days",
	"health_threshold_failing",
	"health_threshold_warning",
	"max_heatmap_runs",
	"default_hook_timeout_seconds",
	"file_browser_blocked_paths",
	"command_timeout_backup_seconds",
	"command_timeout_restore_seconds",
	"command_timeout_list_snapshots_seconds",
	"command_timeout_browse_snapshot_seconds",
	"command_timeout_browse_filesystem_seconds",
	"command_timeout_default_seconds",
	"outbox_spill_max_rows",
	"outbox_spill_retention_seconds",
	"outbox_flush_interval_seconds",
	"outbox_delivery_timeout_seconds",
	"outbox_max_attempts",
}

func getSettingsHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings := make(map[string]json.RawMessage)

		for _, key := range settingsKeys {
			val, err := db.GetSetting(r.Context(), key)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if val != nil {
				settings[key] = json.RawMessage(*val)
			}
		}

		writeJSON(w, http.StatusOK, settings)
	}
}

// allowedSettings is the set of keys accepted by the update handler.
var allowedSettings = func() map[string]bool {
	m := make(map[string]bool, len(settingsKeys))
	for _, k := range settingsKeys {
		m[k] = true
	}
	return m
}()

func updateSettingsHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		for key := range input {
			if !allowedSettings[key] {
				writeError(w, http.StatusBadRequest, "unknown setting: "+key)
				return
			}
		}

		for key, value := range input {
			if err := db.SetSetting(r.Context(), key, string(value)); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}

		// Global settings change affects all agents.
		go resolver.PushConfigToAllAgents(context.Background())

		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}
