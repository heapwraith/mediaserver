package api

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"mediaserver/internal/library"
)

func isNotFound(err error) bool { return errors.Is(err, library.ErrNotFound) }

// sessionBody answers a profile choice: the token goes in the X-Profile header.
type sessionBody struct {
	Profile library.Profile `json:"profile"`
	Token   string          `json:"token"`
}

func (d Deps) session(p library.Profile, pinHash string) sessionBody {
	return sessionBody{Profile: p, Token: sessionToken(d.Secret, p.ID, pinHash)}
}

// GET /api/profiles
func (d Deps) listProfiles(w http.ResponseWriter, r *http.Request) {
	list, err := d.Library.Profiles(r.Context())
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, list)
}

// POST /api/profiles: the new profile starts a session right away.
func (d Deps) createProfile(w http.ResponseWriter, r *http.Request) {
	var in library.ProfileInput
	if err := decodeJSON(r, &in); err != nil {
		d.writeError(w, r, err)
		return
	}
	p, err := d.Library.CreateProfile(r.Context(), in)
	if err == nil {
		var hash string
		if _, hash, err = d.Library.ProfileAuth(r.Context(), p.ID); err == nil {
			writeJSON(w, http.StatusCreated, d.session(p, hash))
			return
		}
	}
	d.writeError(w, r, err)
}

type pinBody struct {
	PIN string `json:"pin,omitempty"`
}

// POST /api/profiles/{id}/session: picks a profile, checking its PIN.
func (d Deps) startSession(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	var b pinBody
	if err := decodeJSON(r, &b); err != nil {
		d.writeError(w, r, err)
		return
	}
	if left := d.pins.wait(id); left > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(left.Seconds()))))
		writeJSON(w, http.StatusTooManyRequests, errorBody{Error: "too many wrong PINs, wait a moment", Code: "pin_locked"})
		return
	}
	p, hash, err := d.Library.ProfileAuth(r.Context(), id)
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	if hash != "" && !library.VerifyPIN(hash, b.PIN) {
		if d.pins.fail(id) {
			w.Header().Set("Retry-After", strconv.Itoa(int(pinLock.Seconds())))
			writeJSON(w, http.StatusTooManyRequests, errorBody{Error: "too many wrong PINs, wait a moment", Code: "pin_locked"})
			return
		}
		writeJSON(w, http.StatusForbidden, errorBody{Error: "wrong PIN", Code: "wrong_pin"})
		return
	}
	d.pins.reset(id)
	writeJSON(w, http.StatusOK, d.session(p, hash))
}

// ownProfile reads the {id} of a profile route; only the profile of the
// session may change or delete itself.
func (d Deps) ownProfile(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, err := pathID(r, "id")
	if err != nil {
		d.writeError(w, r, err)
		return "", false
	}
	if profileFrom(r.Context()).ID != id {
		writeJSON(w, http.StatusForbidden, errorBody{Error: "choose this profile first", Code: "forbidden"})
		return "", false
	}
	return id, true
}

// PUT /api/profiles/{id}: answers a new token (a PIN change ends the old one).
func (d Deps) updateProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := d.ownProfile(w, r)
	if !ok {
		return
	}
	var in library.ProfileInput
	if err := decodeJSON(r, &in); err != nil {
		d.writeError(w, r, err)
		return
	}
	p, err := d.Library.UpdateProfile(r.Context(), id, in)
	if err == nil {
		var hash string
		if _, hash, err = d.Library.ProfileAuth(r.Context(), id); err == nil {
			writeJSON(w, http.StatusOK, d.session(p, hash))
			return
		}
	}
	d.writeError(w, r, err)
}

// DELETE /api/profiles/{id}
func (d Deps) deleteProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := d.ownProfile(w, r)
	if !ok {
		return
	}
	if err := d.Library.DeleteProfile(r.Context(), id); err != nil {
		d.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/progress/{mediaId}
func (d Deps) getProgress(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "mediaId")
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	p, err := d.Library.ProgressOf(r.Context(), profileFrom(r.Context()).ID, id)
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, p)
}

type progressBody struct {
	PositionMs int64 `json:"position_ms"`
	DurationMs int64 `json:"duration_ms"`
}

// PUT /api/progress/{mediaId}
func (d Deps) putProgress(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "mediaId")
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	var b progressBody
	if err := decodeJSON(r, &b); err != nil {
		d.writeError(w, r, err)
		return
	}
	p, err := d.Library.SaveProgress(r.Context(), profileFrom(r.Context()).ID, id, b.PositionMs, b.DurationMs)
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// PUT /api/watched
func (d Deps) putWatched(w http.ResponseWriter, r *http.Request) {
	var t library.WatchedTarget
	if err := decodeJSON(r, &t); err != nil {
		d.writeError(w, r, err)
		return
	}
	ids := append([]string(nil), t.MediaIDs...)
	for _, id := range []string{t.SeasonID, t.SeriesID} {
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) > library.MaxLimit {
		d.writeError(w, r, fmt.Errorf("%w: at most %d media ids", library.ErrInvalid, library.MaxLimit))
		return
	}
	if err := validIDs(ids...); err != nil {
		d.writeError(w, r, err)
		return
	}
	if err := d.Library.SetWatched(r.Context(), profileFrom(r.Context()).ID, t); err != nil {
		d.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/series/{id}/progress
func (d Deps) seriesProgress(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	p, err := d.Library.SeriesProgressOf(r.Context(), profileFrom(r.Context()).ID, id)
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, p)
}

// GET /api/home: the profile rows of the home page.
func (d Deps) home(w http.ResponseWriter, r *http.Request) {
	h, err := d.Library.HomeOf(r.Context(), profileFrom(r.Context()).ID)
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, h)
}
