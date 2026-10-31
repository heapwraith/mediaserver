package api

import (
	"bytes"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"
)

// spaHandler serves the built frontend. Existing files are served as is;
// any other path without a file extension falls back to index.html so the
// client-side router can handle it.
type spaHandler struct {
	fsys    fs.FS
	files   http.Handler
	index   []byte
	modTime time.Time
}

func newSPAHandler(fsys fs.FS, logger *slog.Logger) http.Handler {
	h := &spaHandler{fsys: fsys, modTime: time.Now()}
	if fsys != nil {
		h.files = http.FileServerFS(fsys)
		index, err := fs.ReadFile(fsys, "index.html")
		if err == nil {
			h.index = index
		}
	}
	if h.index == nil {
		logger.Warn("frontend build not found: run `make build` (serving API only)")
	}
	return h
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.index == nil {
		http.Error(w, "frontend not built", http.StatusNotFound)
		return
	}

	// path.Clean on a rooted path removes every "..": nothing outside fsys.
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name != "" && name != "index.html" {
		if info, err := fs.Stat(h.fsys, name); err == nil && !info.IsDir() {
			if strings.HasPrefix(name, "_app/immutable/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			h.files.ServeHTTP(w, r)
			return
		}
		// A missing asset (has an extension) is a real 404, not a page.
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", h.modTime, bytes.NewReader(h.index))
}
