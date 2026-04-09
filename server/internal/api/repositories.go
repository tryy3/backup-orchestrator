package api

import (
	"context"
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

		repos, err := db.ListRepositories(r.Context(), scope, agentID)
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

		if !validRepoScopes[repo.Scope] {
			writeError(w, http.StatusBadRequest, "scope must be 'global' or 'local'")
			return
		}

		if repo.Scope == "local" && repo.AgentID == nil {
			writeError(w, http.StatusBadRequest, "agent_id is required for local-scoped repositories")
			return
		}

		if err := db.CreateRepository(r.Context(), &repo); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// New repos don't affect existing plans yet, but if local-scoped the agent might need to know.
		if repo.Scope == "local" && repo.AgentID != nil {
			go func() {
				if err := resolver.PushConfigToAgent(context.Background(), *repo.AgentID); err != nil {
					log.Printf("failed to push config to agent %s after repo create: %v", *repo.AgentID, err)
				}
			}()
		}

		writeJSON(w, http.StatusCreated, repo)
	}
}

func getRepositoryHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		repo, err := db.GetRepository(r.Context(), id)
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

		if err := db.UpdateRepository(r.Context(), &repo); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		// Push config to all agents whose plans reference this repo.
		go pushConfigToAgentsUsingRepo(context.Background(), db, resolver, id)

		writeJSON(w, http.StatusOK, repo)
	}
}

func deleteRepositoryHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		// Find affected agents before deleting.
		agentIDs, err := db.AgentIDsUsingRepository(r.Context(), id)
		if err != nil {
			log.Printf("Failed to find agents using repository %s: %v", id, err)
		}

		if err := db.DeleteRepository(r.Context(), id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		// Push config to affected agents (repo removed from their config).
		for _, agentID := range agentIDs {
			agentID := agentID
			go func() {
				if err := resolver.PushConfigToAgent(context.Background(), agentID); err != nil {
					log.Printf("failed to push config to agent %s after repo delete: %v", agentID, err)
				}
			}()
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func pushConfigToAgentsUsingRepo(ctx context.Context, db *database.DB, resolver *configpush.Resolver, repoID string) {
	agentIDs, err := db.AgentIDsUsingRepository(ctx, repoID)
	if err != nil {
		log.Printf("error finding agents for repo %s: %v", repoID, err)
		return
	}
	for _, agentID := range agentIDs {
		if err := resolver.PushConfigToAgent(ctx, agentID); err != nil {
			log.Printf("failed to push config to agent %s for repo %s: %v", agentID, repoID, err)
		}
	}
}
