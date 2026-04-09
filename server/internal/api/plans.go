package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"

	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
	"github.com/tryy3/backup-orchestrator/server/internal/events"
)

func listPlansHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := r.URL.Query().Get("agent_id")
		plans, err := db.ListPlans(r.Context(), agentID)
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

		if !isValidCronSchedule(p.Schedule) {
			writeError(w, http.StatusBadRequest, "schedule must be a valid cron expression (5 or 6 fields)")
			return
		}

		if len(p.RepositoryIDs) == 0 {
			writeError(w, http.StatusBadRequest, "at least one repository_id is required")
			return
		}

		if err := db.CreatePlan(r.Context(), &p); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Push updated config to agent.
		go func() {
			if err := resolver.PushConfigToAgent(context.Background(), p.AgentID); err != nil {
				log.Printf("failed to push config to agent %s after plan create: %v", p.AgentID, err)
			}
		}()

		writeJSON(w, http.StatusCreated, p)
	}
}

func getPlanHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		p, err := db.GetPlan(r.Context(), id)
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

		if err := db.UpdatePlan(r.Context(), &p); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		// Push updated config to agent.
		go func() {
			if err := resolver.PushConfigToAgent(context.Background(), p.AgentID); err != nil {
				log.Printf("failed to push config to agent %s after plan update: %v", p.AgentID, err)
			}
		}()

		writeJSON(w, http.StatusOK, p)
	}
}

func deletePlanHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		// Get the plan first to know which agent to notify.
		plan, err := db.GetPlan(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if plan == nil {
			writeError(w, http.StatusNotFound, "plan not found")
			return
		}

		if err := db.DeletePlan(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Push updated config to agent (plan removed).
		go func() {
			if err := resolver.PushConfigToAgent(context.Background(), plan.AgentID); err != nil {
				log.Printf("failed to push config to agent %s after plan delete: %v", plan.AgentID, err)
			}
		}()

		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func triggerPlanHandler(db *database.DB, cmdr AgentCommander, hub *events.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		plan, err := db.GetPlan(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if plan == nil {
			writeError(w, http.StatusNotFound, "plan not found")
			return
		}

		if !cmdr.IsOnline(plan.AgentID) {
			writeError(w, http.StatusBadGateway, "agent not connected")
			return
		}

		// Create a planned job immediately so it's visible in the UI.
		plannedJob, err := db.CreatePlannedJob(r.Context(), plan.AgentID, id, plan.Name, "manual")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Broadcast job.created event.
		hub.Broadcast(events.Event{
			Type:    "job.created",
			Payload: plannedJob,
		})

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
			"job_id":  plannedJob.ID,
		})
	}
}
