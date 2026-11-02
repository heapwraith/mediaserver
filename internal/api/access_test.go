// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

const noSession = "X-Profile: "

func TestLibraryNeedsASession(t *testing.T) {
	srv, sc, fileID := newLibraryServer(t, "Filmes/The Lattice (1999)/The Lattice (1999).mkv")
	if _, err := sc.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	id := fileID("Filmes/The Lattice (1999)/The Lattice (1999).mkv")
	for _, path := range []string{
		"/api/movies", "/api/series", "/api/genres", "/api/library/scan", "/api/media/" + id,
		"/api/media/" + id + "/stream", "/api/images/" + id, "/api/media/" + id + "/fonts",
		"/api/subtitles/" + id + "/vtt", "/api/review",
	} {
		if status, _ := do(t, srv, http.MethodGet, path, "", noSession); status != http.StatusUnauthorized {
			t.Errorf("GET %s without a session = %d, want 401", path, status)
		}
	}
	// What stays open: the health check, the profile list and the PIN screens.
	for _, path := range []string{"/api/health", "/api/profiles", "/api/admin/access"} {
		if status, _ := do(t, srv, http.MethodGet, path, "", noSession); status != http.StatusOK {
			t.Errorf("GET %s without a session = %d, want 200", path, status)
		}
	}
}

func TestMediaToken(t *testing.T) {
	srv, sc, fileID := newLibraryServer(t, "Filmes/The Lattice (1999)/The Lattice (1999).mkv")
	if _, err := sc.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	id := fileID("Filmes/The Lattice (1999)/The Lattice (1999).mkv")
	call := func(m, p, b string, h ...string) (int, string) { return do(t, srv, m, p, b, append(h, noSession)...) }
	_, header := newProfile(t, call, "Sala", "")

	status, body := do(t, srv, http.MethodGet, "/api/media-token", "", header)
	if status != http.StatusOK {
		t.Fatalf("media-token = %d %s", status, body)
	}
	raw := decode[mediaTokenBody](t, body).Token
	tok := url.QueryEscape(raw)

	if status, _ := call(http.MethodGet, "/api/media/"+id+"/stream?m="+tok, ""); status != http.StatusOK {
		t.Errorf("stream with a media token = %d, want 200", status)
	}
	if status, _ := call(http.MethodGet, "/api/media/"+id+"/stream?m=junk", ""); status != http.StatusUnauthorized {
		t.Errorf("stream with a junk token = %d, want 401", status)
	}
	// The weak token reads media; it does not write anything.
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/library/scan?m=" + tok, ""},
		{http.MethodPut, "/api/watched?m=" + tok, `{}`},
		{http.MethodPut, "/api/progress/" + id + "?m=" + tok, `{"position_ms":1,"duration_ms":2}`},
	} {
		if status, _ := call(c.method, c.path, c.body); status != http.StatusUnauthorized {
			t.Errorf("%s %s with a media token = %d, want 401", c.method, c.path, status)
		}
	}
	// A session token is not a media token, and the reverse.
	sessionTok := url.QueryEscape(strings.TrimPrefix(header, ProfileHeader+": "))
	if status, _ := call(http.MethodGet, "/api/media/"+id+"/stream?m="+sessionTok, ""); status != http.StatusUnauthorized {
		t.Errorf("a session token worked as a media token: %d", status)
	}
	if status, _ := call(http.MethodGet, "/api/home", "", ProfileHeader+": "+raw); status != http.StatusUnauthorized {
		t.Errorf("a media token worked as a session token: %d", status)
	}
}

func TestMediaTokenFunctions(t *testing.T) {
	secret := []byte("0123456789abcdef")
	now := time.Unix(1_000_000, 0)
	tok := mediaToken(secret, "p1", "hash", now)
	if !validMediaToken(secret, tok, "p1", "hash", now) {
		t.Fatal("fresh token refused")
	}
	if tok != mediaToken(secret, "p1", "hash", now.Add(time.Hour)) {
		t.Error("the token changes within the same window: image URLs would not be cacheable")
	}
	if validMediaToken(secret, tok, "p1", "hash", now.Add(mediaTTL+mediaStep)) {
		t.Error("expired token accepted")
	}
	if validMediaToken(secret, tok, "p1", "other", now) || validMediaToken(secret, tok, "p2", "hash", now) {
		t.Error("token accepted for another PIN or profile")
	}
	if validMediaToken(secret, sessionToken(secret, "p1", "hash", now.Add(time.Hour)), "p1", "hash", now) {
		t.Error("a session token is accepted as media token")
	}
}

func TestAdminGatesProfilesAndStatus(t *testing.T) {
	srv, _, _ := newLibraryServer(t)
	call := func(m, p, b string, h ...string) (int, string) { return do(t, srv, m, p, b, append(h, noSession)...) }
	_, header := newProfile(t, call, "Dono", "")
	if status, body := do(t, srv, http.MethodPut, "/api/admin/pin", `{"pin":"4321"}`, header); status != http.StatusOK {
		t.Fatalf("set admin PIN = %d %s", status, body)
	}
	visita := `{"name":"Visita","color":"#3E63DD"}`
	if status, body := call(http.MethodPost, "/api/profiles", visita); status != http.StatusForbidden || !strings.Contains(body, "admin_required") {
		t.Errorf("create a profile with an admin PIN set = %d %s, want 403 admin_required", status, body)
	}
	status, body := call(http.MethodPost, "/api/admin/unlock", `{"pin":"4321"}`)
	if status != http.StatusOK {
		t.Fatalf("unlock = %d %s", status, body)
	}
	admin := AdminHeader + ": " + decode[adminTokenBody](t, body).Token
	if status, _ := call(http.MethodPost, "/api/profiles", visita, admin); status != http.StatusCreated {
		t.Errorf("create a profile with the admin token = %d, want 201", status)
	}
	if status, _ := do(t, srv, http.MethodGet, "/api/admin/status", "", header); status != http.StatusForbidden {
		t.Errorf("status without the admin token = %d, want 403", status)
	}
	if status, _ := do(t, srv, http.MethodGet, "/api/admin/status", "", header, admin); status != http.StatusOK {
		t.Errorf("status with the admin token = %d, want 200", status)
	}
}

func TestRevokeSessions(t *testing.T) {
	srv, _, _ := newLibraryServer(t)
	call := func(m, p, b string, h ...string) (int, string) { return do(t, srv, m, p, b, append(h, noSession)...) }
	_, header := newProfile(t, call, "Sala", "")
	if status, _ := do(t, srv, http.MethodGet, "/api/home", "", header); status != http.StatusOK {
		t.Fatalf("home = %d", status)
	}
	if status, _ := do(t, srv, http.MethodPost, "/api/admin/revoke", "", header); status != http.StatusNoContent {
		t.Fatalf("revoke = %d, want 204", status)
	}
	if status, _ := do(t, srv, http.MethodGet, "/api/home", "", header); status != http.StatusUnauthorized {
		t.Errorf("the old session after revoke = %d, want 401", status)
	}
	_, fresh := newProfile(t, call, "Outro", "")
	if status, _ := do(t, srv, http.MethodGet, "/api/home", "", fresh); status != http.StatusOK {
		t.Errorf("a new session after revoke = %d, want 200", status)
	}
}
