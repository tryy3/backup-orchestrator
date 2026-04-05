package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

func listRepositoriesHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := r.URL.Query().Get("scope")
		agentID := r.URL.Query().Get("agent_id")

		repos, err := db.ListRepositories(scope, agentID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if repos == nil {
			repos = []database.Repository{}
		}
		writeJSON(w, http.StatusOK, repos)
	}
}

func createRepositoryHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var repo database.Repository
		if err := json.NewDecoder(r.Body).Decode(&repo); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if repo.Name == "" || repo.Scope == "" || repo.Type == "" || repo.Path == "" || repo.Password == "" {
			writeError(w, http.StatusBadRequest, "name, scope, type, path, and password are required")
			return
		}

		if err := db.CreateRepository(&repo); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// New repos don't affect existing plans yet, but if local-scoped the agent might need to know.
		if repo.Scope == "local" && repo.AgentID != nil {
			go resolver.PushConfigToAgent(*repo.AgentID)
		}

		writeJSON(w, http.StatusCreated, repo)
	}
}

func getRepositoryHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		repo, err := db.GetRepository(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if repo == nil {
			writeError(w, http.StatusNotFound, "repository not found")
			return
		}
		writeJSON(w, http.StatusOK, repo)
	}
}

func updateRepositoryHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var repo database.Repository
		if err := json.NewDecoder(r.Body).Decode(&repo); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		repo.ID = id

		if err := db.UpdateRepository(&repo); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		// Push config to all agents whose plans reference this repo.
		go pushConfigToAgentsUsingRepo(db, resolver, id)

		writeJSON(w, http.StatusOK, repo)
	}
}

func deleteRepositoryHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		// Find affected agents before deleting.
		agentIDs, _ := db.AgentIDsUsingRepository(id)

		if err := db.DeleteRepository(id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		// Push config to affected agents (repo removed from their config).
		for _, agentID := range agentIDs {
			go resolver.PushConfigToAgent(agentID)
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func pushConfigToAgentsUsingRepo(db *database.DB, resolver *configpush.Resolver, repoID string) {
	agentIDs, err := db.AgentIDsUsingRepository(repoID)
	if err != nil {
		log.Printf("error finding agents for repo %s: %v", repoID, err)
		return
	}
	for _, agentID := range agentIDs {
		resolver.PushConfigToAgent(agentID)
	}
}
