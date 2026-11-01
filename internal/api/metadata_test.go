package api

import (
	"net/http"
	"strings"
	"testing"

	"mediaserver/internal/metadata"
)

func TestMetadataEndpoints(t *testing.T) {
	const secret = "abc123secretkey"
	srv, sc, _, conn := newLibraryServerDB(t, progressFiles...)
	if _, err := sc.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body string, headers ...string) (int, string) {
		return do(t, srv, method, path, body, headers...)
	}
	if status, _ := call(http.MethodGet, "/api/metadata/status", ""); status != http.StatusUnauthorized {
		t.Errorf("status without a profile = %d, want 401", status)
	}
	_, h := newProfile(t, call, "Ana", "")

	// Off by default.
	_, body := call(http.MethodGet, "/api/metadata/status", "", h)
	st := decode[metadata.Status](t, body)
	if st.Enabled || st.HasKey {
		t.Fatalf("default status = %s", body)
	}

	// The key goes in and never comes back.
	status, body := call(http.MethodPut, "/api/metadata/settings", `{"key":"`+secret+`","enabled":true}`, h)
	if status != http.StatusOK || strings.Contains(body, secret) {
		t.Fatalf("settings = %d %s", status, body)
	}
	if st := decode[metadata.Status](t, body); !st.Enabled || !st.HasKey {
		t.Fatalf("status = %s", body)
	}
	if _, body := call(http.MethodGet, "/api/metadata/status", "", h); strings.Contains(body, secret) {
		t.Fatalf("the key came back: %s", body)
	}
	if status, _ := call(http.MethodPut, "/api/metadata/settings", `{"key":"two words"}`, h); status != http.StatusBadRequest {
		t.Errorf("bad key = %d, want 400", status)
	}
	if status, _ := call(http.MethodPost, "/api/metadata/sync", "", h); status != http.StatusAccepted {
		t.Errorf("sync = %d, want 202", status)
	}

	// A title waiting in Revisar: it lists, and "none of these" takes it out.
	var movie string
	if err := conn.QueryRowContext(t.Context(), `SELECT id FROM movies LIMIT 1`).Scan(&movie); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(t.Context(), `
		INSERT INTO title_meta (owner_id, owner_type, status, lookup_key, candidates, updated_at)
		VALUES (?, 'movie', 'review', 'x', '[{"id":7,"title":"Sete","year":1995}]', 1)`, movie); err != nil {
		t.Fatal(err)
	}
	_, body = call(http.MethodGet, "/api/metadata/review", "", h)
	items := decode[[]metadata.ReviewItem](t, body)
	if len(items) != 1 || items[0].ID != movie || items[0].Candidates[0].ID != 7 {
		t.Fatalf("review = %s", body)
	}
	for _, b := range []string{`{}`, `{"tmdb_id":-1}`} {
		if status, _ := call(http.MethodPut, "/api/metadata/"+movie, b, h); status != http.StatusBadRequest {
			t.Errorf("answer %s = %d, want 400", b, status)
		}
	}
	if status, _ := call(http.MethodPut, "/api/metadata/not-a-uuid", `{"skip":true}`, h); status != http.StatusBadRequest {
		t.Errorf("bad id = %d, want 400", status)
	}
	if status, body := call(http.MethodPut, "/api/metadata/"+movie, `{"skip":true}`, h); status != http.StatusNoContent {
		t.Fatalf("skip = %d %s", status, body)
	}
	if _, body := call(http.MethodGet, "/api/metadata/review", "", h); len(decode[[]metadata.ReviewItem](t, body)) != 0 {
		t.Fatalf("review after skip = %s", body)
	}
}
