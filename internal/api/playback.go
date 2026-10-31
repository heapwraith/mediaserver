package api

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	"mediaserver/internal/library"
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
			// What each device got and why: checked on every TV (docs/tv/README.md).
			if d.Logger != nil {
				d.Logger.InfoContext(r.Context(), "playback", "media", id, "direct", pb.Direct,
					"video", pb.Video, "audio", pb.Audio, "burned", pb.Burned, "caps", strings.Join(req.Caps, ","))
			}
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
	// An ASS subtitle on a client that cannot run JASSUB is burned in.
	if req.Subtitle != "" && !slices.Contains(req.Caps, "ass") {
		i := slices.IndexFunc(m.Subtitles, func(s library.SubtitleInfo) bool { return s.ID == req.Subtitle })
		if i < 0 {
			return stream.Playback{}, fmt.Errorf("%w: unknown subtitle %s", library.ErrInvalid, req.Subtitle)
		}
		if m.Subtitles[i].Kind == "ass" {
			if d.Subtitles == nil {
				return stream.Playback{}, fmt.Errorf("%w: subtitles not available", library.ErrInvalid)
			}
			dir, file, err := d.Subtitles.BurnFile(r.Context(), req.Subtitle)
			if err != nil {
				return stream.Playback{}, err
			}
			req.Burn = &stream.Burn{Dir: dir, File: file}
		}
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
