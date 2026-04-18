package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

func listScriptsHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scripts, err := db.ListScripts(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if scripts == nil {
			scripts = []database.Script{}
		}
		writeJSON(w, http.StatusOK, scripts)
	}
}

func createScriptHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var s database.Script
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if s.Name == "" || s.Command == "" {
			writeError(w, http.StatusBadRequest, "name and command are required")
			return
		}

		if err := db.CreateScript(r.Context(), &s); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// New scripts don't affect any plans yet (not referenced by any hooks).
		writeJSON(w, http.StatusCreated, s)
	}
}

func getScriptHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		s, err := db.GetScript(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if s == nil {
			writeError(w, http.StatusNotFound, "script not found")
			return
		}
		writeJSON(w, http.StatusOK, s)
	}
}

func updateScriptHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var s database.Script
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		s.ID = id

		if err := db.UpdateScript(r.Context(), &s); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		// Push config to all agents whose plans reference this script.
		go pushConfigToAgentsUsingScript(context.Background(), db, resolver, id)

		writeJSON(w, http.StatusOK, s)
	}
}

func deleteScriptHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := db.DeleteScript(r.Context(), id); err != nil {
			// Check if the error is about references.
			if strings.Contains(err.Error(), "referenced by") {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		// No config push needed: delete is blocked if script is still referenced.
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func pushConfigToAgentsUsingScript(ctx context.Context, db *database.DB, resolver *configpush.Resolver, scriptID string) {
	agentIDs, err := db.AgentIDsUsingScript(ctx, scriptID)
	if err != nil {
		slog.Error("error finding agents for script", "script_id", scriptID, "error", err)
		return
	}
	for _, agentID := range agentIDs {
		if err := resolver.PushConfigToAgent(ctx, agentID); err != nil {
			slog.Error("failed to push config to agent for script", "agent_id", agentID, "script_id", scriptID, "error", err)
		}
	}
}
