package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"mediaserver/internal/library"
)

var progressFiles = []string{
	"Filmes/Alien (1979)/Alien (1979).mkv",
	"Series/Dark/Season 1/Dark - S01E01.mkv",
	"Series/Dark/Season 1/Dark - S01E02.mkv",
	"Series/Dark/Season 1/Dark - S01E03.mkv",
}

// newProfile creates a profile and returns its X-Profile header.
func newProfile(t *testing.T, call func(method, path, body string, headers ...string) (int, string), name, pin string) (library.Profile, string) {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q,"color":"#3E63DD"}`, name)
	if pin != "" {
		body = fmt.Sprintf(`{"name":%q,"color":"#3E63DD","pin":%q}`, name, pin)
	}
	status, res := call(http.MethodPost, "/api/profiles", body)
	if status != http.StatusCreated {
		t.Fatalf("create %s = %d %s", name, status, res)
	}
	s := decode[sessionBody](t, res)
	return s.Profile, ProfileHeader + ": " + s.Token
}

func TestProfilesAndProgress(t *testing.T) {
	srv, sc, fileID, conn := newLibraryServerDB(t, progressFiles...)
	if _, err := sc.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body string, headers ...string) (int, string) {
		return do(t, srv, method, path, body, headers...)
	}
	alien := fileID(progressFiles[0])
	ep1, ep2, ep3 := fileID(progressFiles[1]), fileID(progressFiles[2]), fileID(progressFiles[3])
	// The test files are not probed: give them a duration.
	for _, id := range []string{alien, ep1, ep2, ep3} {
		if _, err := conn.ExecContext(t.Context(), `UPDATE media_files SET duration_ms = 600000 WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
	}

	ana, anaH := newProfile(t, call, "Ana", "")
	bia, biaH := newProfile(t, call, "Bia", "1234")
	if ana.HasPIN || !bia.HasPIN {
		t.Fatalf("has_pin: ana %v, bia %v", ana.HasPIN, bia.HasPIN)
	}
	if status, _ := call(http.MethodPost, "/api/profiles", `{"name":"ana","color":"#000000"}`); status != http.StatusConflict {
		t.Errorf("duplicate name (case) = %d, want 409", status)
	}
	if status, _ := call(http.MethodPost, "/api/profiles", `{"name":"Caio","color":"red"}`); status != http.StatusBadRequest {
		t.Errorf("bad color = %d, want 400", status)
	}
	if status, body := call(http.MethodGet, "/api/profiles", ""); status != http.StatusOK || strings.Contains(body, "pin_hash") ||
		len(decode[[]library.Profile](t, body)) != 2 {
		t.Fatalf("profiles = %d %s", status, body)
	}

	// No session, a forged one, or another profile's one.
	for _, h := range []string{"", ProfileHeader + ": " + ana.ID, ProfileHeader + ": " + ana.ID + ".AAAA", ProfileHeader + ": junk"} {
		if status, _ := call(http.MethodGet, "/api/home", "", h); status != http.StatusUnauthorized {
			t.Errorf("home with %q = %d, want 401", h, status)
		}
	}
	if status, _ := call(http.MethodDelete, "/api/profiles/"+ana.ID, "", biaH); status != http.StatusForbidden {
		t.Errorf("delete another profile = %d, want 403", status)
	}

	// PIN: no PIN, wrong PIN, right PIN; Ana has no PIN.
	if status, _ := call(http.MethodPost, "/api/profiles/"+bia.ID+"/session", `{}`); status != http.StatusForbidden {
		t.Errorf("session without pin = %d", status)
	}
	if status, body := call(http.MethodPost, "/api/profiles/"+bia.ID+"/session", `{"pin":"1234"}`); status != http.StatusOK ||
		ProfileHeader+": "+decode[sessionBody](t, body).Token != biaH {
		t.Errorf("session with pin = %d %s", status, body)
	}
	if status, _ := call(http.MethodPost, "/api/profiles/"+ana.ID+"/session", `{}`); status != http.StatusOK {
		t.Errorf("session without pin (none set) = %d", status)
	}

	// Ana watches 2 min of the movie and finishes episode 1.
	if status, body := call(http.MethodPut, "/api/progress/"+alien, `{"position_ms":120000,"duration_ms":1}`, anaH); status != http.StatusOK ||
		decode[library.Progress](t, body).DurationMs != 600000 {
		t.Fatalf("save progress = %d %s (the probed duration wins)", status, body)
	}
	if status, body := call(http.MethodPut, "/api/progress/"+ep1, `{"position_ms":550000,"duration_ms":600000}`, anaH); status != http.StatusOK ||
		!decode[library.Progress](t, body).Watched {
		t.Fatalf("90%% = %d %s, want watched", status, body)
	}
	_, body := call(http.MethodGet, "/api/progress/"+alien, "", anaH)
	if p := decode[library.Progress](t, body); p.PositionMs != 120000 || p.Watched {
		t.Fatalf("progress = %s", body)
	}

	_, body = call(http.MethodGet, "/api/home", "", anaH)
	h := decode[library.Home](t, body)
	if len(h.Continue) != 1 || h.Continue[0].MediaID != alien || h.Continue[0].Kind != "movie" {
		t.Fatalf("continue = %s", body)
	}
	if len(h.Next) != 1 || h.Next[0].MediaID != ep2 || h.Next[0].Title != "Dark" || h.Next[0].Episode.Number != 2 {
		t.Fatalf("next = %s", body)
	}

	// Acceptance: a profile's progress never shows in another one.
	_, body = call(http.MethodGet, "/api/home", "", biaH)
	if h := decode[library.Home](t, body); len(h.Continue) != 0 || len(h.Next) != 0 {
		t.Fatalf("bia home = %s", body)
	}
	_, body = call(http.MethodGet, "/api/progress/"+alien, "", biaH)
	if p := decode[library.Progress](t, body); p.PositionMs != 0 {
		t.Fatalf("bia progress = %s", body)
	}

	// Series progress and up next.
	_, body = call(http.MethodGet, "/api/media/"+ep1+"/next", "")
	if next := decode[library.HomeItem](t, body); next.MediaID != ep2 {
		t.Fatalf("next of ep1 = %s", body)
	}
	if status, _ := call(http.MethodGet, "/api/media/"+ep3+"/next", ""); status != http.StatusNotFound {
		t.Errorf("next of last = %d, want 404", status)
	}
	var seriesID string
	if err := conn.QueryRowContext(t.Context(), `SELECT series_id FROM episodes WHERE media_file_id = ?`, ep1).Scan(&seriesID); err != nil {
		t.Fatal(err)
	}
	_, body = call(http.MethodGet, "/api/series/"+seriesID+"/progress", "", anaH)
	if sp := decode[library.SeriesProgress](t, body); sp.UpNext != ep2 || !sp.Episodes[ep1].Watched {
		t.Fatalf("series progress = %s", body)
	}

	// Mark the whole series, then unmark one episode.
	if status, body := call(http.MethodPut, "/api/watched", `{"series_id":"`+seriesID+`","watched":true}`, anaH); status != http.StatusNoContent {
		t.Fatalf("mark series = %d %s", status, body)
	}
	if status, _ := call(http.MethodPut, "/api/watched", `{"media_ids":["`+ep2+`"],"watched":false}`, anaH); status != http.StatusNoContent {
		t.Fatalf("unmark = %d", status)
	}
	_, body = call(http.MethodGet, "/api/series/"+seriesID+"/progress", "", anaH)
	if sp := decode[library.SeriesProgress](t, body); len(sp.Episodes) != 2 || sp.UpNext != ep2 {
		t.Fatalf("after marks = %s", body)
	}
	for _, b := range []string{`{"watched":true}`, `{"series_id":"x","watched":true}`, `{"series_id":"` + seriesID + `","season_id":"` + seriesID + `","watched":true}`} {
		if status, _ := call(http.MethodPut, "/api/watched", b, anaH); status != http.StatusBadRequest {
			t.Errorf("watched %s = %d, want 400", b, status)
		}
	}

	// Changing the PIN ends the old session; the answer brings a new one.
	status, body := call(http.MethodPut, "/api/profiles/"+bia.ID, `{"name":"Bia","color":"#3e63dd","pin":"9999"}`, biaH)
	if status != http.StatusOK {
		t.Fatalf("update = %d %s", status, body)
	}
	newBiaH := ProfileHeader + ": " + decode[sessionBody](t, body).Token
	if status, _ := call(http.MethodGet, "/api/home", "", biaH); status != http.StatusUnauthorized {
		t.Errorf("old session after pin change = %d, want 401", status)
	}
	if status, _ := call(http.MethodGet, "/api/home", "", newBiaH); status != http.StatusOK {
		t.Errorf("new session = %d", status)
	}

	// Deleting a profile ends its session.
	if status, _ := call(http.MethodDelete, "/api/profiles/"+ana.ID, "", anaH); status != http.StatusNoContent {
		t.Fatalf("delete = %d", status)
	}
	if status, _ := call(http.MethodGet, "/api/home", "", anaH); status != http.StatusUnauthorized {
		t.Errorf("deleted profile session = %d, want 401", status)
	}
}

func TestPinLockout(t *testing.T) {
	srv, _, _ := newLibraryServer(t)
	call := func(method, path, body string, headers ...string) (int, string) {
		return do(t, srv, method, path, body, headers...)
	}
	p, _ := newProfile(t, call, "Caio", "4321")
	for i := range pinMaxFails {
		status, _ := call(http.MethodPost, "/api/profiles/"+p.ID+"/session", `{"pin":"0000"}`)
		want := http.StatusForbidden
		if i == pinMaxFails-1 {
			want = http.StatusTooManyRequests
		}
		if status != want {
			t.Fatalf("attempt %d = %d, want %d", i+1, status, want)
		}
	}
	// Locked: even the right PIN waits.
	if status, _ := call(http.MethodPost, "/api/profiles/"+p.ID+"/session", `{"pin":"4321"}`); status != http.StatusTooManyRequests {
		t.Fatalf("locked = %d, want 429", status)
	}
}

func TestPinLimiterUnlocks(t *testing.T) {
	now := time.Unix(0, 0)
	l := newPinLimiter()
	l.now = func() time.Time { return now }
	for range pinMaxFails {
		l.fail("p")
	}
	if l.wait("p") != pinLock {
		t.Fatalf("wait = %v", l.wait("p"))
	}
	now = now.Add(pinLock)
	if l.wait("p") != 0 || l.fail("p") {
		t.Fatal("lock did not end, or the count did not restart")
	}
}

func TestProfilesUnavailable(t *testing.T) {
	srv := newTestServer(t, true) // no store, no secret
	if status, _ := do(t, srv, http.MethodGet, "/api/profiles", ""); status != http.StatusServiceUnavailable {
		t.Errorf("profiles = %d, want 503", status)
	}
}
