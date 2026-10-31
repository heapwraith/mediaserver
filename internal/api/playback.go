package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"mediaserver/internal/stream"
)

// requireTranscoder answers 503 when playback sessions are not available.
func (d Deps) requireTranscoder(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Transcoder == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "transcoding not available"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// POST /api/media/{id}/playback {caps, audio, start}: direct play or HLS,
// decided from the codecs the client declares.
func (d Deps) playback(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	var req stream.PlayRequest
	if err == nil {
		err = decodeJSON(r, &req)
	}
	if err == nil {
		var pb stream.Playback
		if pb, err = d.startPlayback(r, id, req); err == nil {
			writeJSON(w, http.StatusOK, pb)
			return
		}
	}
	switch {
	case errors.Is(err, stream.ErrBusy):
		writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "limite de conversões simultâneas atingido"})
	case errors.Is(err, stream.ErrClosed):
		writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "servidor encerrando"})
	default:
		d.writeError(w, r, err)
	}
}

func (d Deps) startPlayback(r *http.Request, id string, req stream.PlayRequest) (stream.Playback, error) {
	m, err := d.Library.MediaByID(r.Context(), id)
	if err != nil {
		return stream.Playback{}, err
	}
	rel, err := d.Library.PlayablePath(r.Context(), id)
	if err != nil {
		return stream.Playback{}, err
	}
	return d.Transcoder.Start(r.Context(), m, rel, req)
}

// GET /api/transcode/{key}/{file}: playlist and segments of a session.
func (d Deps) transcodeFile(w http.ResponseWriter, r *http.Request) {
	if err := d.Transcoder.ServeFile(w, r, chi.URLParam(r, "key"), chi.URLParam(r, "file")); err != nil {
		d.writeError(w, r, err)
	}
}

// DELETE /api/transcode/{key}: the player left or moved to another session.
func (d Deps) stopTranscode(w http.ResponseWriter, r *http.Request) {
	d.Transcoder.Stop(chi.URLParam(r, "key"))
	w.WriteHeader(http.StatusNoContent)
}
