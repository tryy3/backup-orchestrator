package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

func listJobsHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := r.URL.Query().Get("agent_id")
		planID := r.URL.Query().Get("plan_id")
		status := r.URL.Query().Get("status")

		jobs, err := db.ListJobs(agentID, planID, status)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if jobs == nil {
			jobs = []database.Job{}
		}
		writeJSON(w, http.StatusOK, jobs)
	}
}

func getJobHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		job, err := db.GetJob(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if job == nil {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeJSON(w, http.StatusOK, job)
	}
}
