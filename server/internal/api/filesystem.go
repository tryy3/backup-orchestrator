package api

import (
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"
)

func browseFilesystemHandler(cmdr AgentCommander) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := chi.URLParam(r, "id")
		fsPath := r.URL.Query().Get("path")

		if fsPath == "" {
			fsPath = "/"
		}

		// Server-side validation (defense in depth — agent also validates).
		fsPath = path.Clean(fsPath)
		if !strings.HasPrefix(fsPath, "/") {
			writeError(w, http.StatusBadRequest, "path must be absolute")
			return
		}
		if len(fsPath) > 4096 {
			writeError(w, http.StatusBadRequest, "path too long")
			return
		}

		if !cmdr.IsOnline(agentID) {
			writeError(w, http.StatusBadGateway, "agent not connected")
			return
		}

		cmd := &backupv1.Command{
			CommandId: uuid.New().String(),
			Action: &backupv1.Command_BrowseFilesystem{
				BrowseFilesystem: &backupv1.BrowseFilesystem{
					Path: fsPath,
				},
			},
		}

		result, err := cmdr.SendCommand(agentID, cmd)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !result.Success {
			writeError(w, http.StatusInternalServerError, result.Error)
			return
		}

		w.WriteHeader(http.StatusOK)
		if len(result.Data) > 0 {
			_, _ = w.Write(result.Data)
		} else {
			_, _ = w.Write([]byte("[]"))
		}
	}
}
