package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"

	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

// agentResponse is the API response DTO that omits rclone_config and exposes
// has_rclone_config instead.
type agentResponse struct {
	ID              string                      `json:"id"`
	Name            string                      `json:"name"`
	Hostname        string                      `json:"hostname"`
	OS              *string                     `json:"os,omitempty"`
	Status          string                      `json:"status"`
	APIKey          *string                     `json:"api_key,omitempty"`
	AgentVersion    *string                     `json:"agent_version,omitempty"`
	ResticVersion   *string                     `json:"restic_version,omitempty"`
	RcloneVersion   *string                     `json:"rclone_version,omitempty"`
	HasRcloneConfig bool                        `json:"has_rclone_config"`
	LastHeartbeat   *time.Time                  `json:"last_heartbeat,omitempty"`
	LastJobAt       *time.Time                  `json:"last_job_at,omitempty"`
	ConfigVersion   int                         `json:"config_version"`
	ConfigAppliedAt *time.Time                  `json:"config_applied_at,omitempty"`
	CommandTimeouts *configpush.CommandTimeouts `json:"command_timeouts,omitempty"`
	CreatedAt       time.Time                   `json:"created_at"`
	UpdatedAt       time.Time                   `json:"updated_at"`
}

func toAgentResponse(a *database.Agent) agentResponse {
	resp := agentResponse{
		ID:              a.ID,
		Name:            a.Name,
		Hostname:        a.Hostname,
		OS:              a.OS,
		Status:          a.Status,
		APIKey:          a.APIKey,
		AgentVersion:    a.AgentVersion,
		ResticVersion:   a.ResticVersion,
		RcloneVersion:   a.RcloneVersion,
		HasRcloneConfig: a.RcloneConfig != nil && *a.RcloneConfig != "",
		LastHeartbeat:   a.LastHeartbeat,
		LastJobAt:       a.LastJobAt,
		ConfigVersion:   a.ConfigVersion,
		ConfigAppliedAt: a.ConfigAppliedAt,
		CreatedAt:       a.CreatedAt,
		UpdatedAt:       a.UpdatedAt,
	}
	if a.CommandTimeouts != nil && *a.CommandTimeouts != "" {
		var ct configpush.CommandTimeouts
		if err := json.Unmarshal([]byte(*a.CommandTimeouts), &ct); err == nil {
			resp.CommandTimeouts = &ct
		}
	}
	return resp
}

func listAgentsHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agents, err := db.ListAgents(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp := make([]agentResponse, 0, len(agents))
		for i := range agents {
			resp = append(resp, toAgentResponse(&agents[i]))
		}
		writeJSON(w, http.StatusOK, resp)
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
		writeJSON(w, http.StatusOK, toAgentResponse(agent))
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
					slog.Error("failed to push config to agent after approval", "agent_id", id, "error", err)
				}
			}()
		}

		agent, err := db.GetAgent(r.Context(), id)
		if err != nil {
			slog.Error("failed to reload agent after approval", "agent_id", id, "error", err)
		}
		writeJSON(w, http.StatusOK, toAgentResponse(agent))
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
			slog.Error("failed to reload agent after rejection", "agent_id", id, "error", err)
		}
		writeJSON(w, http.StatusOK, toAgentResponse(agent))
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

func getRcloneHandler(db *database.DB) http.HandlerFunc {
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
		config := ""
		if agent.RcloneConfig != nil {
			config = *agent.RcloneConfig
		}
		writeJSON(w, http.StatusOK, map[string]string{"rclone_config": config})
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

		// Blank config keeps existing value — the rclone update only touches
		// the rclone_config column, so skipping the write preserves the stored
		// value (unlike repository updates which rewrite all columns).
		if input.RcloneConfig != "" {
			if err := db.UpdateRcloneConfig(r.Context(), id, input.RcloneConfig); err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
		}

		// Push updated config to agent.
		go func() {
			if err := resolver.PushConfigToAgent(context.Background(), id); err != nil {
				slog.Error("failed to push config to agent after rclone update", "agent_id", id, "error", err)
			}
		}()

		agent, err := db.GetAgent(r.Context(), id)
		if err != nil {
			slog.Error("failed to reload agent after rclone update", "agent_id", id, "error", err)
		}
		writeJSON(w, http.StatusOK, toAgentResponse(agent))
	}
}

// updateAgentCommandTimeoutsHandler stores per-agent overrides of the global
// command timeout settings. Sending null or an empty body clears the override
// (the agent will fall back to the global settings or its compiled-in defaults).
func updateAgentCommandTimeoutsHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		// Accept either a CommandTimeouts object or null to clear the override.
		var input *configpush.CommandTimeouts
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		// Validate: each provided value must be non-negative.
		if input != nil {
			vals := []int32{
				input.BackupSecs, input.RestoreSecs, input.ListSnapshotsSecs,
				input.BrowseSnapshotSecs, input.BrowseFilesystemSecs, input.DefaultSecs,
			}
			for _, v := range vals {
				if v < 0 {
					writeError(w, http.StatusBadRequest, "command timeout values must be non-negative")
					return
				}
			}
		}

		var stored *string
		if input != nil {
			b, err := json.Marshal(input)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			s := string(b)
			stored = &s
		}

		if err := db.UpdateCommandTimeouts(r.Context(), id, stored); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		// Push updated config so the agent picks up the new timeouts immediately.
		go func() {
			if err := resolver.PushConfigToAgent(context.Background(), id); err != nil {
				slog.Error("failed to push config to agent after command timeouts update", "agent_id", id, "error", err)
			}
		}()

		agent, err := db.GetAgent(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, toAgentResponse(agent))
	}
}
