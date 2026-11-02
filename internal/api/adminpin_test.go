// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAdminPin(t *testing.T) {
	srv, _, _ := newLibraryServer(t)
	call := func(method, path, body string, headers ...string) (int, string) {
		return do(t, srv, method, path, body, headers...)
	}
	_, profile := newProfile(t, call, "Sala", "")
	guarded := []struct{ method, path, body string }{
		{http.MethodPost, "/api/library/scan", ""},
		{http.MethodGet, "/api/review", ""},
		{http.MethodPost, "/api/series/merge", `{"into":"a","from":["b"]}`},
		{http.MethodGet, "/api/admin/backup", ""},
	}

	// Without an admin PIN nothing changes: the administration is open.
	if status, body := call(http.MethodGet, "/api/admin/access", ""); status != 200 || !strings.Contains(body, `"enabled":false`) || !strings.Contains(body, `"unlocked":true`) {
		t.Fatalf("access = %d %s", status, body)
	}
	if status, _ := call(http.MethodGet, "/api/review", ""); status != http.StatusOK {
		t.Fatalf("review without admin PIN = %d", status)
	}

	// Setting it needs a profile session.
	if status, _ := call(http.MethodPut, "/api/admin/pin", `{"pin":"4321"}`, "X-Profile: "); status != http.StatusUnauthorized {
		t.Fatalf("set without profile = %d, want 401", status)
	}
	if status, _ := call(http.MethodPut, "/api/admin/pin", `{"pin":"12"}`, profile); status != http.StatusBadRequest {
		t.Fatalf("short PIN = %d, want 400", status)
	}
	status, body := call(http.MethodPut, "/api/admin/pin", `{"pin":"4321"}`, profile)
	if status != 200 {
		t.Fatalf("set = %d %s", status, body)
	}
	first := decode[adminTokenBody](t, body).Token

	// Now the administration asks for the token.
	for _, g := range guarded {
		status, body := call(g.method, g.path, g.body, profile)
		if status != http.StatusForbidden || !strings.Contains(body, "admin_required") {
			t.Errorf("%s %s without token = %d %s, want 403 admin_required", g.method, g.path, status, body)
		}
		if status, _ := call(g.method, g.path, g.body, profile, AdminHeader+": junk"); status != http.StatusForbidden {
			t.Errorf("%s %s with junk token = %d, want 403", g.method, g.path, status)
		}
	}
	if status, _ := call(http.MethodGet, "/api/review", "", AdminHeader+": "+first); status != http.StatusOK {
		t.Errorf("review with the token = %d, want 200", status)
	}
	// Playback and the library stay open to every profile.
	if status, _ := call(http.MethodGet, "/api/movies", ""); status != http.StatusOK {
		t.Errorf("movies = %d", status)
	}
	if status, body := call(http.MethodGet, "/api/admin/access", "", AdminHeader+": "+first); !strings.Contains(body, `"enabled":true`) || !strings.Contains(body, `"unlocked":true`) || status != 200 {
		t.Errorf("access with token = %s", body)
	}
	if _, body := call(http.MethodGet, "/api/admin/access", ""); !strings.Contains(body, `"unlocked":false`) {
		t.Errorf("access without token = %s", body)
	}
	// Changing it needs the token too.
	if status, _ := call(http.MethodPut, "/api/admin/pin", `{"pin":"1111"}`, profile); status != http.StatusForbidden {
		t.Errorf("change without token = %d, want 403", status)
	}

	// Unlock: wrong PIN, then right.
	if status, _ := call(http.MethodPost, "/api/admin/unlock", `{"pin":"0000"}`); status != http.StatusForbidden {
		t.Errorf("wrong PIN = %d, want 403", status)
	}
	status, body = call(http.MethodPost, "/api/admin/unlock", `{"pin":"4321"}`)
	if status != 200 || decode[adminTokenBody](t, body).Token == "" {
		t.Fatalf("unlock = %d %s", status, body)
	}

	// A new PIN ends the old token.
	status, body = call(http.MethodPut, "/api/admin/pin", `{"pin":"1111"}`, profile, AdminHeader+": "+first)
	if status != 200 {
		t.Fatalf("change = %d %s", status, body)
	}
	if status, _ := call(http.MethodGet, "/api/review", "", AdminHeader+": "+first); status != http.StatusForbidden {
		t.Errorf("old token after a PIN change = %d, want 403", status)
	}
	second := decode[adminTokenBody](t, body).Token

	// Empty PIN removes it.
	if status, _ := call(http.MethodPut, "/api/admin/pin", `{"pin":""}`, profile, AdminHeader+": "+second); status != 200 {
		t.Fatalf("remove = %d", status)
	}
	if status, _ := call(http.MethodGet, "/api/review", ""); status != http.StatusOK {
		t.Errorf("review after removing the PIN = %d, want 200", status)
	}
}

func TestAdminTokenExpires(t *testing.T) {
	secret := []byte("0123456789abcdef")
	now := time.Unix(1_000_000, 0)
	tok := adminToken(secret, "hash", now.Add(time.Minute))
	if !validAdminToken(secret, tok, "hash", now) {
		t.Fatal("fresh token refused")
	}
	if validAdminToken(secret, tok, "hash", now.Add(time.Hour)) || validAdminToken(secret, tok, "other", now) || validAdminToken(nil, tok, "hash", now) {
		t.Error("expired token, other PIN or empty secret accepted")
	}
	// A profile session token is not an admin token.
	if validAdminToken(secret, sessionToken(secret, "p", "hash", now.Add(time.Hour)), "hash", now) {
		t.Error("a profile token works as admin token")
	}
}
