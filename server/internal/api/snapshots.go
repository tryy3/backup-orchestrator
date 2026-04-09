package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"
)

func listSnapshotsHandler(cmdr AgentCommander) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := chi.URLParam(r, "id")
		repoID := r.URL.Query().Get("repo")

		if repoID == "" {
			writeError(w, http.StatusBadRequest, "repo query parameter is required")
			return
		}

		if !cmdr.IsOnline(agentID) {
			writeError(w, http.StatusBadGateway, "agent not connected")
			return
		}

		cmd := &backupv1.Command{
			CommandId: uuid.New().String(),
			Action: &backupv1.Command_ListSnapshots{
				ListSnapshots: &backupv1.ListSnapshots{
					RepositoryId: repoID,
				},
			},
		}

		result, err := cmdr.SendCommand(agentID, cmd)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !result.Success {
			writeError(w, http.StatusInternalServerError, result.Error)
			return
		}

		// Return the raw JSON data from the agent.
		w.WriteHeader(http.StatusOK)
		if len(result.Data) > 0 {
			_, _ = w.Write(result.Data)
		} else {
			_, _ = w.Write([]byte("[]"))
		}
	}
}

func browseSnapshotHandler(cmdr AgentCommander) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := chi.URLParam(r, "id")

		var input struct {
			RepositoryID string `json:"repository_id"`
			SnapshotID   string `json:"snapshot_id"`
			Path         string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if !cmdr.IsOnline(agentID) {
			writeError(w, http.StatusBadGateway, "agent not connected")
			return
		}

		cmd := &backupv1.Command{
			CommandId: uuid.New().String(),
			Action: &backupv1.Command_BrowseSnapshot{
				BrowseSnapshot: &backupv1.BrowseSnapshot{
					RepositoryId: input.RepositoryID,
					SnapshotId:   input.SnapshotID,
					Path:         input.Path,
				},
			},
		}

		result, err := cmdr.SendCommand(agentID, cmd)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !result.Success {
			writeError(w, http.StatusInternalServerError, result.Error)
			return
		}

		w.WriteHeader(http.StatusOK)
		if len(result.Data) > 0 {
			_, _ = w.Write(result.Data)
		} else {
			_, _ = w.Write([]byte("[]"))
		}
	}
}

func triggerRestoreHandler(cmdr AgentCommander) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := chi.URLParam(r, "id")

		var input struct {
			RepositoryID string   `json:"repository_id"`
			SnapshotID   string   `json:"snapshot_id"`
			Paths        []string `json:"paths"`
			Target       string   `json:"target"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if !cmdr.IsOnline(agentID) {
			writeError(w, http.StatusBadGateway, "agent not connected")
			return
		}

		cmd := &backupv1.Command{
			CommandId: uuid.New().String(),
			Action: &backupv1.Command_TriggerRestore{
				TriggerRestore: &backupv1.TriggerRestore{
					RepositoryId: input.RepositoryID,
					SnapshotId:   input.SnapshotID,
					Paths:        input.Paths,
					Target:       input.Target,
				},
			},
		}

		result, err := cmdr.SendCommand(agentID, cmd)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": result.Success,
			"error":   result.Error,
		})
	}
}
