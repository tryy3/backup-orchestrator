package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
	"github.com/tryy3/backup-orchestrator/server/internal/settings"
)

func getSettingsHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resolved, err := settings.LoadResolved(r.Context(), db)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resolved)
	}
}

func updateSettingsHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		errs := settings.Validate(input)
		if len(errs) > 0 {
			slog.Warn("settings validation failed", "error_count", len(errs), "errors", errs)
			writeJSON(w, http.StatusBadRequest, map[string]any{"errors": errs})
			return
		}
		if err := settings.Apply(r.Context(), db, input); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		go resolver.PushConfigToAllAgents(context.Background())
		resolved, err := settings.LoadResolved(r.Context(), db)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resolved)
	}
}
