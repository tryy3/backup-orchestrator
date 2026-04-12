//go:build dev

package frontend

import "net/http"

// Handler returns a dev-mode stub that redirects frontend routes to the Vite
// dev server running on :5173. The API is served normally by the server on
// its own port; only direct browser hits to server:8080/* are redirected.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://localhost:5173"+r.URL.RequestURI(), http.StatusTemporaryRedirect)
	})
}
