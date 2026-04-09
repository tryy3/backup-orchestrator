package frontend

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed dist/*
var distFS embed.FS

// Handler returns an http.Handler that serves the embedded Vue.js SPA.
// Non-file requests (no extension or not found) fall back to index.html for SPA routing.
func Handler() http.Handler {
	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("frontend: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Try to serve the file directly.
		if path != "/" && !strings.HasSuffix(path, "/") {
			// Check if the file exists in the embedded FS.
			if f, err := dist.Open(strings.TrimPrefix(path, "/")); err == nil {
				_ = f.Close()
				// Remove the JSON content-type set by middleware for static files.
				w.Header().Del("Content-Type")
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// SPA fallback: serve index.html for all other routes.
		w.Header().Del("Content-Type")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			http.Error(w, "frontend not built", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(data)
	})
}
