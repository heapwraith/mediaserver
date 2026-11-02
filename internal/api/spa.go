// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"regexp"
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
	csp     string
}

func newSPAHandler(fsys fs.FS, logger *slog.Logger) http.Handler {
	h := &spaHandler{fsys: fsys, modTime: time.Now()}
	if fsys != nil {
		h.files = http.FileServerFS(fsys)
		index, err := fs.ReadFile(fsys, "index.html")
		if err == nil {
			h.index = index
			h.csp = contentSecurityPolicy(index)
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
			if name == "index.html" {
				w.Header().Set("Content-Security-Policy", h.csp)
			}
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

	w.Header().Set("Content-Security-Policy", h.csp)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", h.modTime, bytes.NewReader(h.index))
}

var inlineScript = regexp.MustCompile(`(?s)<script>(.*?)</script>`)

// contentSecurityPolicy is the policy of the interface: only our own files,
// plus the one inline script SvelteKit writes into index.html (allowed by its
// hash, so an injected script would not run). 'unsafe-eval' is for JASSUB:
// before Chromium 97 (Tizen 6.5 is Chromium 85) WebAssembly.Module needs it on
// the main thread, and JASSUB tests WebAssembly there; newer browsers would
// only need 'wasm-unsafe-eval', which old ones reject. Measured on the TV.
// style-src keeps 'unsafe-inline' because Svelte sets style attributes.
// frame-ancestors allows only the TV apps (file:// pages).
func contentSecurityPolicy(index []byte) string {
	inline := inlineScript.FindAllSubmatch(index, -1)
	scripts := append(make([]string, 0, 2+len(inline)), "'self'", "'unsafe-eval'")
	for _, m := range inline {
		sum := sha256.Sum256(m[1])
		scripts = append(scripts, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src " + strings.Join(scripts, " "),
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob:",
		"media-src 'self' blob:",
		"font-src 'self' data: blob:",
		"connect-src 'self'",
		"worker-src 'self' blob:",
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors 'self' file:",
	}, "; ")
}
