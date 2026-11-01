package api

import (
	"net/http"
)

// requireMetadata answers 503 when the TMDB lookups are not wired.
func (d Deps) requireMetadata(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Metadata == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "metadata not available", Code: "unavailable"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GET /api/metadata/status. It never has the key.
func (d Deps) metadataStatus(w http.ResponseWriter, r *http.Request) {
	st, err := d.Metadata.Status(r.Context())
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, st)
}

type metadataSettingsBody struct {
	Enabled *bool `json:"enabled,omitempty"`
	// Key replaces the TMDB key; "" removes it.
	Key *string `json:"key,omitempty"`
}

// PUT /api/metadata/settings
func (d Deps) putMetadataSettings(w http.ResponseWriter, r *http.Request) {
	var b metadataSettingsBody
	if err := decodeJSON(r, &b); err != nil {
		d.writeError(w, r, err)
		return
	}
	if b.Key != nil {
		if err := d.Metadata.SetKey(r.Context(), *b.Key); err != nil {
			d.writeError(w, r, err)
			return
		}
	}
	if b.Enabled != nil {
		if err := d.Metadata.SetEnabled(r.Context(), *b.Enabled); err != nil {
			d.writeError(w, r, err)
			return
		}
	}
	d.Metadata.Trigger()
	d.metadataStatus(w, r)
}

// POST /api/metadata/sync
func (d Deps) syncMetadata(w http.ResponseWriter, _ *http.Request) {
	d.Metadata.Trigger()
	w.WriteHeader(http.StatusAccepted)
}

// GET /api/metadata/review
func (d Deps) metadataReview(w http.ResponseWriter, r *http.Request) {
	items, err := d.Metadata.Review(r.Context())
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, items)
}

type metadataAnswerBody struct {
	TMDBID int  `json:"tmdb_id,omitzero"`
	Skip   bool `json:"skip,omitzero"`
}

// PUT /api/metadata/{id}: the user's answer for a title in Revisar.
func (d Deps) answerMetadata(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	var b metadataAnswerBody
	if err := decodeJSON(r, &b); err != nil {
		d.writeError(w, r, err)
		return
	}
	if b.Skip {
		err = d.Metadata.Skip(r.Context(), id)
	} else {
		err = d.Metadata.Choose(r.Context(), id, b.TMDBID)
	}
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
