package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"

	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

func listPlansHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := r.URL.Query().Get("agent_id")
		plans, err := db.ListPlans(agentID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if plans == nil {
			plans = []database.BackupPlan{}
		}
		writeJSON(w, http.StatusOK, plans)
	}
}

func createPlanHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p database.BackupPlan
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if p.Name == "" || p.AgentID == "" || len(p.Paths) == 0 || p.Schedule == "" {
			writeError(w, http.StatusBadRequest, "name, agent_id, paths, and schedule are required")
			return
		}

		if err := db.CreatePlan(&p); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Push updated config to agent.
		go resolver.PushConfigToAgent(p.AgentID)

		writeJSON(w, http.StatusCreated, p)
	}
}

func getPlanHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		p, err := db.GetPlan(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if p == nil {
			writeError(w, http.StatusNotFound, "plan not found")
			return
		}
		writeJSON(w, http.StatusOK, p)
	}
}

func updatePlanHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var p database.BackupPlan
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		p.ID = id

		if err := db.UpdatePlan(&p); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		// Push updated config to agent.
		go resolver.PushConfigToAgent(p.AgentID)

		writeJSON(w, http.StatusOK, p)
	}
}

func deletePlanHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		// Get the plan first to know which agent to notify.
		plan, err := db.GetPlan(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if plan == nil {
			writeError(w, http.StatusNotFound, "plan not found")
			return
		}

		if err := db.DeletePlan(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Push updated config to agent (plan removed).
		go resolver.PushConfigToAgent(plan.AgentID)

		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func triggerPlanHandler(db *database.DB, cmdr AgentCommander) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		plan, err := db.GetPlan(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if plan == nil {
			writeError(w, http.StatusNotFound, "plan not found")
			return
		}

		if !cmdr.IsOnline(plan.AgentID) {
			writeError(w, http.StatusNotFound, "agent not connected")
			return
		}

		cmd := &backupv1.Command{
			Action: &backupv1.Command_TriggerBackup{
				TriggerBackup: &backupv1.TriggerBackup{
					PlanId: id,
				},
			},
		}
		result, err := cmdr.SendCommand(plan.AgentID, cmd)
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
