// Package web serves the built admin dashboard (React SPA) from the embedded
// dist directory. Run `make web-build` (Vite) to regenerate internal/web/dist
// before a Go build; a placeholder index.html is committed so `go build`
// always succeeds even before the frontend has been built.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed dist
var distFS embed.FS

// Handler returns an http.Handler that serves the SPA. API routes are
// registered on the admin mux BEFORE this catch-all, so any path beginning
// with /admin/* or an exact API path takes precedence. Every other GET path
// falls through to index.html to support client-side routing.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// Cannot happen: dist is embedded and present.
		panic(err)
	}

	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/admin") || strings.HasPrefix(path, "/v1") {
			// Let the mux return 404 for unknown API paths instead of the SPA.
			http.NotFound(w, r)
			return
		}
		if path == "/" || !exists(sub, strings.TrimPrefix(path, "/")) {
			// Serve the SPA entrypoint directly. http.FileServer would treat
			// "*/index.html" as a directory index and redirect it to "./",
			// which breaks client-side routes, so bypass it here.
			idx, err := fs.ReadFile(sub, "index.html")
			if err != nil {
				http.Error(w, "index.html missing", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(idx)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func exists(sub fs.FS, name string) bool {
	if name == "" {
		return false
	}
	_, err := fs.Stat(sub, name)
	return err == nil
}
