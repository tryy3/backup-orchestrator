package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"

	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
	"github.com/tryy3/backup-orchestrator/server/internal/events"
	"github.com/tryy3/backup-orchestrator/server/internal/frontend"
)

// AgentCommander defines the interface for sending commands to connected agents.
type AgentCommander interface {
	Send(agentID string, msg *backupv1.ServerMessage) error
	SendCommand(agentID string, cmd *backupv1.Command) (*backupv1.CommandResult, error)
	IsOnline(agentID string) bool
}

// NewRouter creates and configures the Chi HTTP router with all API routes.
// allowedOrigins is the list of origins permitted for CORS requests; only
// those origins will receive CORS response headers.
func NewRouter(db *database.DB, cmdr AgentCommander, resolver *configpush.Resolver, hub *events.Hub, allowedOrigins []string) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(newCORSMiddleware(allowedOrigins))
	r.Use(maxBytesMiddleware(1 << 20)) // 1 MB request body limit

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// WebSocket endpoint (outside /api group to avoid JSON content-type middleware).
	r.Get("/api/ws", websocketHandler(hub))

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Use(jsonContentType)

		// Version info
		r.Get("/version", versionHandler())
		// Agents
		r.Get("/agents", listAgentsHandler(db))
		r.Get("/agents/{id}", getAgentHandler(db))
		r.Post("/agents/{id}/approve", approveAgentHandler(db, cmdr, resolver))
		r.Post("/agents/{id}/reject", rejectAgentHandler(db, cmdr))
		r.Delete("/agents/{id}", deleteAgentHandler(db))
		r.Get("/agents/{id}/rclone", getRcloneHandler(db))
		r.Put("/agents/{id}/rclone", updateRcloneHandler(db, resolver))
		r.Put("/agents/{id}/command-timeouts", updateAgentCommandTimeoutsHandler(db, resolver))
		r.Put("/agents/{id}/outbox-overrides", updateAgentOutboxOverridesHandler(db, resolver))

		// Repositories
		r.Get("/repositories", listRepositoriesHandler(db))
		r.Post("/repositories", createRepositoryHandler(db, resolver))
		r.Get("/repositories/{id}", getRepositoryHandler(db))
		r.Put("/repositories/{id}", updateRepositoryHandler(db, resolver))
		r.Delete("/repositories/{id}", deleteRepositoryHandler(db, resolver))

		// Scripts
		r.Get("/scripts", listScriptsHandler(db))
		r.Post("/scripts", createScriptHandler(db))
		r.Get("/scripts/{id}", getScriptHandler(db))
		r.Put("/scripts/{id}", updateScriptHandler(db, resolver))
		r.Delete("/scripts/{id}", deleteScriptHandler(db))

		// Plans
		r.Get("/plans", listPlansHandler(db))
		r.Post("/plans", createPlanHandler(db, resolver))
		r.Get("/plans/{id}", getPlanHandler(db))
		r.Put("/plans/{id}", updatePlanHandler(db, resolver))
		r.Delete("/plans/{id}", deletePlanHandler(db, resolver))
		r.Post("/plans/{id}/trigger", triggerPlanHandler(db, cmdr, hub))

		// Hooks
		r.Get("/plans/{id}/hooks", listHooksHandler(db))
		r.Post("/plans/{id}/hooks", createHookHandler(db, resolver))
		r.Put("/plans/{id}/hooks/reorder", reorderHooksHandler(db, resolver))
		r.Put("/plans/{id}/hooks/{hid}", updateHookHandler(db, resolver))
		r.Delete("/plans/{id}/hooks/{hid}", deleteHookHandler(db, resolver))

		// Jobs
		r.Get("/jobs", listJobsHandler(db))
		r.Get("/jobs/{id}", getJobHandler(db))

		// Snapshots (agent commands)
		r.Get("/agents/{id}/snapshots", listSnapshotsHandler(cmdr))
		r.Post("/agents/{id}/snapshots/browse", browseSnapshotHandler(cmdr))
		r.Post("/agents/{id}/restore", triggerRestoreHandler(cmdr))
		r.Get("/agents/{id}/fs", browseFilesystemHandler(cmdr))

		// Settings
		r.Get("/settings", getSettingsHandler(db))
		r.Put("/settings", updateSettingsHandler(db, resolver))
	})

	// Serve embedded frontend SPA for all non-API routes.
	r.NotFound(frontend.Handler().ServeHTTP)

	return r
}

// newCORSMiddleware returns a middleware that enforces CORS policy based on
// the provided allowedOrigins list. CORS response headers (Allow-Origin,
// Allow-Methods, Allow-Headers) are only emitted when the request's Origin
// header matches one of the allowed values. Non-matching origins receive no
// CORS headers and pre-flight OPTIONS requests are answered with 204 only when
// the origin is allowed.
func newCORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	// Build a fast lookup set.
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			originAllowed := false
			if origin != "" {
				if _, ok := allowed[origin]; ok {
					originAllowed = true
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
					// Vary must be set so caches don't serve one origin's response to another.
					w.Header().Add("Vary", "Origin")
				}
			}

			if r.Method == http.MethodOptions {
				// Reject pre-flight from disallowed origins so the server fails
				// fast and the response is unambiguous (no CORS headers + 403).
				// Pre-flight without an Origin header (e.g. non-browser tooling)
				// is passed through normally.
				if origin != "" && !originAllowed {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// jsonContentType sets the Content-Type header to application/json.
func jsonContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// maxBytesMiddleware limits the size of incoming request bodies.
func maxBytesMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// writeJSON encodes a value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON encode error", "error", err)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// isValidCronSchedule checks that s looks like a cron expression (5 or 6 space-separated fields).
func isValidCronSchedule(s string) bool {
	fields := strings.Fields(s)
	return len(fields) == 5 || len(fields) == 6
}

// validHookEvents is the set of accepted on_event values for plan hooks.
var validHookEvents = map[string]bool{
	"pre_backup":   true,
	"post_backup":  true,
	"on_success":   true,
	"on_failure":   true,
	"pre_restore":  true,
	"post_restore": true,
	"pre_forget":   true,
	"post_forget":  true,
}

// validRepoScopes is the set of accepted scope values for repositories.
var validRepoScopes = map[string]bool{
	"global": true,
	"local":  true,
}
