package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

func listHooksHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		planID := chi.URLParam(r, "id")
		hooks, err := db.ListHooks(planID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if hooks == nil {
			hooks = []database.PlanHook{}
		}
		writeJSON(w, http.StatusOK, hooks)
	}
}

func createHookHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		planID := chi.URLParam(r, "id")

		var h database.PlanHook
		if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		h.PlanID = planID

		if h.OnEvent == "" {
			writeError(w, http.StatusBadRequest, "on_event is required")
			return
		}

		if err := db.CreateHook(&h); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Push config to the plan's agent.
		pushConfigForPlan(db, resolver, planID)

		writeJSON(w, http.StatusCreated, h)
	}
}

func updateHookHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		planID := chi.URLParam(r, "id")
		hookID := chi.URLParam(r, "hid")

		var h database.PlanHook
		if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		h.ID = hookID
		h.PlanID = planID

		if err := db.UpdateHook(&h); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		pushConfigForPlan(db, resolver, planID)

		writeJSON(w, http.StatusOK, h)
	}
}

func deleteHookHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		planID := chi.URLParam(r, "id")
		hookID := chi.URLParam(r, "hid")

		if err := db.DeleteHook(hookID); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		pushConfigForPlan(db, resolver, planID)

		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func reorderHooksHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		planID := chi.URLParam(r, "id")

		var input struct {
			HookIDs []string `json:"hook_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if len(input.HookIDs) == 0 {
			writeError(w, http.StatusBadRequest, "hook_ids is required")
			return
		}

		if err := db.ReorderHooks(planID, input.HookIDs); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		pushConfigForPlan(db, resolver, planID)

		hooks, _ := db.ListHooks(planID)
		if hooks == nil {
			hooks = []database.PlanHook{}
		}
		writeJSON(w, http.StatusOK, hooks)
	}
}

// pushConfigForPlan looks up the plan's agent and triggers a config push.
func pushConfigForPlan(db *database.DB, resolver *configpush.Resolver, planID string) {
	plan, err := db.GetPlan(planID)
	if err != nil || plan == nil {
		return
	}
	go resolver.PushConfigToAgent(plan.AgentID)
}
