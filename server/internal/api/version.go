package api

import (
	"net/http"

	"github.com/tryy3/backup-orchestrator/server/internal/version"
)

// versionResponse is the JSON payload returned by the version endpoint.
type versionResponse struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

func versionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, versionResponse{
			Version:   version.Version,
			Commit:    version.Commit,
			BuildDate: version.BuildDate,
		})
	}
}
