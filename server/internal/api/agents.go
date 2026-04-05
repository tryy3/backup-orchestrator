package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"

	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

func listAgentsHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agents, err := db.ListAgents()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if agents == nil {
			agents = []database.Agent{}
		}
		writeJSON(w, http.StatusOK, agents)
	}
}

func getAgentHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		agent, err := db.GetAgent(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if agent == nil {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		writeJSON(w, http.StatusOK, agent)
	}
}

func approveAgentHandler(db *database.DB, cmdr AgentCommander, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		// Generate a secure API key.
		keyBytes := make([]byte, 32)
		if _, err := rand.Read(keyBytes); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate API key")
			return
		}
		apiKey := hex.EncodeToString(keyBytes)

		if err := db.ApproveAgent(id, apiKey); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		// Send approval to connected agent.
		if cmdr.IsOnline(id) {
			msg := &backupv1.ServerMessage{
				Payload: &backupv1.ServerMessage_Approval{
					Approval: &backupv1.Approval{
						Status: backupv1.AgentStatus_AGENT_STATUS_APPROVED,
						ApiKey: apiKey,
					},
				},
			}
			_ = cmdr.Send(id, msg)

			// Push initial config to the newly approved agent.
			go resolver.PushConfigToAgent(id)
		}

		agent, _ := db.GetAgent(id)
		writeJSON(w, http.StatusOK, agent)
	}
}

func rejectAgentHandler(db *database.DB, cmdr AgentCommander) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		if err := db.RejectAgent(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		// Send rejection to connected agent.
		if cmdr.IsOnline(id) {
			msg := &backupv1.ServerMessage{
				Payload: &backupv1.ServerMessage_Approval{
					Approval: &backupv1.Approval{
						Status: backupv1.AgentStatus_AGENT_STATUS_REJECTED,
					},
				},
			}
			_ = cmdr.Send(id, msg)
		}

		agent, _ := db.GetAgent(id)
		writeJSON(w, http.StatusOK, agent)
	}
}

func deleteAgentHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := db.DeleteAgent(id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func updateRcloneHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var input struct {
			RcloneConfig string `json:"rclone_config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if err := db.UpdateRcloneConfig(id, input.RcloneConfig); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		// Push updated config to agent.
		go resolver.PushConfigToAgent(id)

		agent, _ := db.GetAgent(id)
		writeJSON(w, http.StatusOK, agent)
	}
}
