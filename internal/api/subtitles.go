package api

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"mediaserver/internal/library"
)

// requireSubtitles answers 503 when subtitles are not available.
func (d Deps) requireSubtitles(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Subtitles == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "subtitles not available", Code: "unavailable"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GET /api/subtitles/{id}/vtt?offset=S: a text subtitle as WebVTT, shifted
// to an HLS session that starts S seconds into the file.
func (d Deps) subtitleVTT(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	var offset float64
	if err == nil {
		if v := r.URL.Query().Get("offset"); v != "" {
			offset, err = strconv.ParseFloat(v, 64)
			if err != nil || offset < 0 || math.IsNaN(offset) || math.IsInf(offset, 0) || offset >= float64(math.MaxInt64)/float64(time.Second) {
				err = fmt.Errorf("%w: offset must be a number of seconds", library.ErrInvalid)
			}
		}
	}
	var data []byte
	if err == nil {
		data, err = d.Subtitles.VTT(r.Context(), id, time.Duration(offset*float64(time.Second)))
	}
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	writeText(w, "text/vtt; charset=utf-8", data)
}

// GET /api/subtitles/{id}/ass: an ASS/SSA subtitle in UTF-8 (JASSUB).
func (d Deps) subtitleASS(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	var data []byte
	if err == nil {
		data, err = d.Subtitles.ASS(r.Context(), id)
	}
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	writeText(w, "text/plain; charset=utf-8", data)
}

func writeText(w http.ResponseWriter, ctype string, data []byte) {
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data) //nolint:gosec // subtitle text with a text content type and nosniff, not HTML
}

// GET /api/media/{id}/fonts: URLs of the fonts attached to the file.
func (d Deps) mediaFonts(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	var names []string
	if err == nil {
		names, err = d.Subtitles.Fonts(r.Context(), id)
	}
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	urls := make([]string, len(names))
	for i, n := range names {
		urls[i] = "/api/media/" + id + "/fonts/" + n
	}
	writeJSON(w, http.StatusOK, urls)
}

var fontTypes = map[string]string{
	".ttf": "font/ttf", ".otf": "font/otf", ".ttc": "font/collection", ".woff": "font/woff", ".woff2": "font/woff2",
}

// GET /api/media/{id}/fonts/{name}
func (d Deps) mediaFont(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	var p string
	if err == nil {
		p, err = d.Subtitles.FontPath(r.Context(), id, chi.URLParam(r, "name"))
	}
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	f, err := os.Open(p) //nolint:gosec // FontPath: cache dir + a name matching subtitles.FontPattern
	if err != nil {
		d.writeError(w, r, fmt.Errorf("font: %w", library.ErrNotFound))
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", fontTypes[filepath.Ext(p)])
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "", info.ModTime(), f)
}
