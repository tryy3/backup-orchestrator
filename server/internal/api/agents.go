package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"

	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

func listAgentsHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agents, err := db.ListAgents(r.Context())
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
		agent, err := db.GetAgent(r.Context(), id)
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

		if err := db.ApproveAgent(r.Context(), id, apiKey); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, "agent not found or not in pending status")
			} else {
				writeError(w, http.StatusInternalServerError, err.Error())
			}
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
			go func() {
				if err := resolver.PushConfigToAgent(context.Background(), id); err != nil {
					log.Printf("failed to push config to agent %s after approval: %v", id, err)
				}
			}()
		}

		agent, err := db.GetAgent(r.Context(), id)
		if err != nil {
			log.Printf("Failed to reload agent %s after approval: %v", id, err)
		}
		writeJSON(w, http.StatusOK, agent)
	}
}

func rejectAgentHandler(db *database.DB, cmdr AgentCommander) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		if err := db.RejectAgent(r.Context(), id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, "agent not found or not in pending status")
			} else {
				writeError(w, http.StatusInternalServerError, err.Error())
			}
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

		agent, err := db.GetAgent(r.Context(), id)
		if err != nil {
			log.Printf("Failed to reload agent %s after rejection: %v", id, err)
		}
		writeJSON(w, http.StatusOK, agent)
	}
}

func deleteAgentHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := db.DeleteAgent(r.Context(), id); err != nil {
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

		if err := db.UpdateRcloneConfig(r.Context(), id, input.RcloneConfig); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		// Push updated config to agent.
		go func() {
			if err := resolver.PushConfigToAgent(context.Background(), id); err != nil {
				log.Printf("failed to push config to agent %s after rclone update: %v", id, err)
			}
		}()

		agent, err := db.GetAgent(r.Context(), id)
		if err != nil {
			log.Printf("Failed to reload agent %s after rclone update: %v", id, err)
		}
		writeJSON(w, http.StatusOK, agent)
	}
}
