package api

import (
	"encoding/json"
	"net/http"

	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

func getSettingsHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings := make(map[string]json.RawMessage)

		retentionVal, err := db.GetSetting("default_retention")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if retentionVal != nil {
			settings["default_retention"] = json.RawMessage(*retentionVal)
		}

		writeJSON(w, http.StatusOK, settings)
	}
}

func updateSettingsHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		for key, value := range input {
			if err := db.SetSetting(key, string(value)); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}

		// Global settings change affects all agents.
		go resolver.PushConfigToAllAgents()

		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}
