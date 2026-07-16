package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// spaHandler serves the built dashboard SPA from a directory, with history-mode
// fallback: unknown non-asset paths return index.html so client-side routing
// works. API and WebSocket routes are registered before this and take priority.
func spaHandler(dir string) http.HandlerFunc {
	fileServer := http.FileServer(http.Dir(dir))
	indexPath := filepath.Join(dir, "index.html")

	return func(w http.ResponseWriter, r *http.Request) {
		// Resolve the requested file within dir, guarding against traversal.
		clean := filepath.Clean(r.URL.Path)
		full := filepath.Join(dir, clean)
		if !strings.HasPrefix(full, filepath.Clean(dir)) {
			http.NotFound(w, r)
			return
		}

		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			// Long-cache fingerprinted assets; never cache index.html.
			if strings.HasPrefix(clean, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback → index.html.
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, indexPath)
	}
}
