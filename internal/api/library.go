package api

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"uuid"

	"github.com/go-chi/chi/v5"

	"mediaserver/internal/library"
	"mediaserver/internal/scanner"
	"mediaserver/internal/stream"
)

const maxBodyBytes = 1 << 20

// decodeJSON reads a small JSON body, rejecting unknown fields.
func decodeJSON(r *http.Request, v any) error {
	data, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("%w: read body: %w", scanner.ErrInvalid, err)
	}
	if err := json.Unmarshal(data, v, json.RejectUnknownMembers(true)); err != nil {
		return fmt.Errorf("%w: invalid JSON: %w", scanner.ErrInvalid, err)
	}
	return nil
}

// writeError maps domain errors to HTTP status codes.
func (d Deps) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		// The client left (e.g. a TV row scrolled past a loading image).
	case errors.Is(err, scanner.ErrInvalid), errors.Is(err, library.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
	case errors.Is(err, library.ErrConflict):
		writeJSON(w, http.StatusConflict, errorBody{Error: err.Error()})
	case errors.Is(err, scanner.ErrNotFound), errors.Is(err, library.ErrNotFound), errors.Is(err, stream.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorBody{Error: "not found"})
	default:
		d.Logger.ErrorContext(r.Context(), "request failed", "path", r.URL.Path, "err", err)
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "internal error"})
	}
}

// pathID reads a UUID URL parameter.
func pathID(r *http.Request, name string) (string, error) {
	id := chi.URLParam(r, name)
	if _, err := uuid.Parse(id); err != nil {
		return "", fmt.Errorf("%w: %s must be a UUID", scanner.ErrInvalid, name)
	}
	return id, nil
}

func validIDs(ids ...string) error {
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return fmt.Errorf("%w: %q is not a UUID", scanner.ErrInvalid, id)
		}
	}
	return nil
}

type scanStartBody struct {
	Started bool           `json:"started"` // false: one was running, another is queued
	Status  scanner.Status `json:"status"`
}

// POST /api/library/scan
func (d Deps) startScan(w http.ResponseWriter, _ *http.Request) {
	started := d.StartScan()
	writeJSON(w, http.StatusAccepted, scanStartBody{Started: started, Status: d.Scanner.Status()})
}

// GET /api/library/scan
func (d Deps) scanStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, d.Scanner.Status())
}

// GET /api/review
func (d Deps) reviewQueue(w http.ResponseWriter, r *http.Request) {
	items, err := d.Scanner.ReviewQueue(r.Context())
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, items)
}

// PUT /api/review/{fileId}
func (d Deps) correctFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "fileId")
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	var c scanner.Correction
	if err := decodeJSON(r, &c); err != nil {
		d.writeError(w, r, err)
		return
	}
	st, err := d.Scanner.Correct(r.Context(), id, c)
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

type mergeBody struct {
	SourceIDs []string `json:"source_ids"`
	TargetID  string   `json:"target_id"`
}

type seriesIDBody struct {
	SeriesID string `json:"series_id"`
}

// POST /api/series/merge
func (d Deps) mergeSeries(w http.ResponseWriter, r *http.Request) {
	var b mergeBody
	if err := decodeJSON(r, &b); err != nil {
		d.writeError(w, r, err)
		return
	}
	if err := validIDs(append(b.SourceIDs, b.TargetID)...); err != nil {
		d.writeError(w, r, err)
		return
	}
	id, err := d.Scanner.MergeSeries(r.Context(), b.SourceIDs, b.TargetID)
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, seriesIDBody{SeriesID: id})
}

type splitBody struct {
	SeriesID string   `json:"series_id"`
	FileIDs  []string `json:"file_ids"`
	Title    string   `json:"title"`
}

// POST /api/series/split
func (d Deps) splitSeries(w http.ResponseWriter, r *http.Request) {
	var b splitBody
	if err := decodeJSON(r, &b); err != nil {
		d.writeError(w, r, err)
		return
	}
	if err := validIDs(append(b.FileIDs, b.SeriesID)...); err != nil {
		d.writeError(w, r, err)
		return
	}
	id, err := d.Scanner.SplitSeries(r.Context(), b.SeriesID, b.FileIDs, b.Title)
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, seriesIDBody{SeriesID: id})
}
