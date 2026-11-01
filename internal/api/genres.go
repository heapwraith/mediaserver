package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"mediaserver/internal/library"
)

// GET /api/genres?kind=movie|series|anime
func (d Deps) listGenres(w http.ResponseWriter, r *http.Request) {
	list, err := d.Library.Genres(r.Context(), r.URL.Query().Get("kind"))
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	writeCachedJSON(w, r, list)
}

// GET /api/genres/rows
func (d Deps) genreRows(w http.ResponseWriter, r *http.Request) {
	rows, err := d.Library.GenreRows(r.Context())
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	writeCachedJSON(w, r, rows)
}

// GET /api/collections
func (d Deps) listCollections(w http.ResponseWriter, r *http.Request) {
	list, err := d.Library.Collections(r.Context())
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	writeCachedJSON(w, r, list)
}

// GET /api/collections/{id}
func (d Deps) collection(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		d.writeError(w, r, fmt.Errorf("%w: id must be a number", library.ErrInvalid))
		return
	}
	c, err := d.Library.CollectionByID(r.Context(), id)
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	writeCachedJSON(w, r, c)
}
