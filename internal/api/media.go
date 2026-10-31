package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"mediaserver/internal/library"
	"mediaserver/internal/scanner"
	"mediaserver/internal/stream"
)

const defaultLimit = 100

// pageParams reads ?sort=title|added|year&order=asc|desc&offset=&limit=&q=.
// q is a local title search, normalized like the stored titles (no accents,
// case or punctuation).
func pageParams(r *http.Request) (library.Page, error) {
	q := r.URL.Query()
	p := library.Page{Sort: q.Get("sort"), Limit: defaultLimit, Query: scanner.NormTitle(q.Get("q"))}
	switch q.Get("order") {
	case "", "asc":
	case "desc":
		p.Desc = true
	default:
		return p, fmt.Errorf("%w: order must be asc or desc", library.ErrInvalid)
	}
	for name, dst := range map[string]*int{"offset": &p.Offset, "limit": &p.Limit} {
		if v := q.Get(name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return p, fmt.Errorf("%w: %s must be a number", library.ErrInvalid, name)
			}
			*dst = n
		}
	}
	return p, nil
}

// writeCachedJSON answers with an ETag of the body; a matching If-None-Match
// gets 304 so clients revalidate listings cheaply.
func writeCachedJSON(w http.ResponseWriter, r *http.Request, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "internal error", Code: "internal"})
		return
	}
	sum := sha256.Sum256(data)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if inm := r.Header.Get("If-None-Match"); inm != "" && (inm == "*" || strings.Contains(inm, etag)) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(data) //nolint:gosec // JSON body with a JSON content type, not HTML
}

// GET /api/movies
func (d Deps) listMovies(w http.ResponseWriter, r *http.Request) {
	p, err := pageParams(r)
	if err == nil {
		var list library.List[library.Movie]
		if list, err = d.Library.Movies(r.Context(), p); err == nil {
			writeCachedJSON(w, r, list)
			return
		}
	}
	d.writeError(w, r, err)
}

// GET /api/series and /api/animes
func (d Deps) listSeries(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := pageParams(r)
		if err == nil {
			var list library.List[library.Series]
			if list, err = d.Library.Series(r.Context(), kind, p); err == nil {
				writeCachedJSON(w, r, list)
				return
			}
		}
		d.writeError(w, r, err)
	}
}

// byID wraps a "load one thing by its {id}" handler.
func byID[T any](d Deps, load func(d Deps, r *http.Request, id string) (T, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r, "id")
		if err == nil {
			var v T
			if v, err = load(d, r, id); err == nil {
				writeCachedJSON(w, r, v)
				return
			}
		}
		d.writeError(w, r, err)
	}
}

// GET /api/media/{id}/stream
func (d Deps) streamMedia(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err == nil {
		err = d.Stream.ServeMedia(w, r, id)
	}
	if err != nil {
		d.writeError(w, r, err)
	}
}

// GET /api/images/{id}?w=480|960|1920&variant=card|backdrop|blur
func (d Deps) image(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	q := r.URL.Query()
	variant := q.Get("variant")
	if variant == "" {
		variant = "card"
	}
	width := 960
	if v := q.Get("w"); v != "" {
		width, _ = strconv.Atoi(v)
	}
	if !slices.Contains(stream.Variants, variant) || !slices.Contains(stream.Widths, width) {
		d.writeError(w, r, fmt.Errorf("%w: variant must be card, backdrop or blur and w 480, 960 or 1920", library.ErrInvalid))
		return
	}
	if err := d.Images.ServeImage(w, r, id, variant, width); err != nil {
		d.writeError(w, r, err)
	}
}
