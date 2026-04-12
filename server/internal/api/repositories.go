package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

// repositoryResponse is the API response DTO that omits the password.
type repositoryResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Scope     string    `json:"scope"`
	AgentID   *string   `json:"agent_id,omitempty"`
	Type      string    `json:"type"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toRepositoryResponse(r *database.Repository) repositoryResponse {
	return repositoryResponse{
		ID:        r.ID,
		Name:      r.Name,
		Scope:     r.Scope,
		AgentID:   r.AgentID,
		Type:      r.Type,
		Path:      r.Path,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func listRepositoriesHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := r.URL.Query().Get("scope")
		agentID := r.URL.Query().Get("agent_id")

		repos, err := db.ListRepositories(r.Context(), scope, agentID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp := make([]repositoryResponse, 0, len(repos))
		for i := range repos {
			resp = append(resp, toRepositoryResponse(&repos[i]))
		}
		writeJSON(w, http.StatusOK, resp)
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

		writeJSON(w, http.StatusCreated, toRepositoryResponse(&repo))
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
		writeJSON(w, http.StatusOK, toRepositoryResponse(repo))
	}
}

func updateRepositoryHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var input struct {
			Name     string  `json:"name"`
			Scope    string  `json:"scope"`
			AgentID  *string `json:"agent_id"`
			Type     string  `json:"type"`
			Path     string  `json:"path"`
			Password string  `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		// If password is blank, keep the existing one.
		if input.Password == "" {
			existing, err := db.GetRepository(r.Context(), id)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if existing == nil {
				writeError(w, http.StatusNotFound, "repository not found")
				return
			}
			input.Password = existing.Password
		}

		repo := &database.Repository{
			ID:       id,
			Name:     input.Name,
			Scope:    input.Scope,
			AgentID:  input.AgentID,
			Type:     input.Type,
			Path:     input.Path,
			Password: input.Password,
		}

		if err := db.UpdateRepository(r.Context(), repo); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		// Push config to all agents whose plans reference this repo.
		go pushConfigToAgentsUsingRepo(context.Background(), db, resolver, id)

		writeJSON(w, http.StatusOK, toRepositoryResponse(repo))
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
