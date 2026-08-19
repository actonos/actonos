package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var embeddedWebAssets embed.FS

// RegisterStaticRoutes mounts the static asset handler on the Chi router.
func (s *Server) RegisterStaticRoutes(overrideDir string) {
	subFS, err := fs.Sub(embeddedWebAssets, "dist")
	if err != nil {
		subFS = embeddedWebAssets
	}

	layered := NewLayeredFS(overrideDir, subFS)
	fileServer := http.FileServer(http.FS(layered))

	s.router.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Skip /api routes
		if strings.HasPrefix(path, "/api") {
			http.NotFound(w, r)
			return
		}

		// Check if file exists, otherwise serve index.html for SPA client-side routing
		f, err := layered.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			_ = f.Close()
			if path == "/sw.js" {
				w.Header().Set("Service-Worker-Allowed", "/")
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fallback to index.html for React SPA
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
