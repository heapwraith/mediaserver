package api

import (
	"encoding/json/v2"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"mediaserver/internal/db"
)

func newTestServer(t *testing.T, withDB bool) *httptest.Server {
	t.Helper()
	d := Deps{
		Logger:  slog.New(slog.DiscardHandler),
		Version: "1.2.3-test",
		Web: fstest.MapFS{
			"index.html":                  {Data: []byte("<!doctype html><title>mediaserver</title>")},
			"favicon.svg":                 {Data: []byte("<svg/>")},
			"_app/immutable/entry/app.js": {Data: []byte("console.log(1)")},
		},
	}
	if withDB {
		conn, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "test.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		d.DB = conn
	}
	srv := httptest.NewServer(NewRouter(d))
	t.Cleanup(srv.Close)
	return srv
}

// get returns the status, headers and body of GET path (the body is closed).
// headers are "Name: value" request headers.
func get(t *testing.T, srv *httptest.Server, path string, headers ...string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range headers {
		k, v, _ := strings.Cut(h, ": ")
		req.Header.Set(k, v)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, res.Header, string(body)
}

func TestHealth(t *testing.T) {
	tests := []struct {
		name       string
		withDB     bool
		wantStatus int
		want       healthBody
	}{
		{"db ok", true, http.StatusOK, healthBody{Status: "ok", Version: "1.2.3-test", DB: "ok"}},
		{"db missing", false, http.StatusServiceUnavailable, healthBody{Status: "degraded", Version: "1.2.3-test", DB: "error"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, header, body := get(t, newTestServer(t, tt.withDB), "/api/health")
			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d", status, tt.wantStatus)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q", ct)
			}
			var got healthBody
			if err := json.Unmarshal([]byte(body), &got); err != nil {
				t.Fatalf("decode %q: %v", body, err)
			}
			if got != tt.want {
				t.Errorf("body = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestStaticAndFallback(t *testing.T) {
	srv := newTestServer(t, true)
	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
		wantCache  string
	}{
		{"root serves index", "/", 200, "<title>mediaserver</title>", "no-cache"},
		{"client route falls back to index", "/series/abc", 200, "<title>mediaserver</title>", "no-cache"},
		{"static file", "/favicon.svg", 200, "<svg/>", "no-cache"},
		{"immutable asset", "/_app/immutable/entry/app.js", 200, "console.log(1)", "public, max-age=31536000, immutable"},
		{"missing asset is 404", "/missing.js", 404, "", ""},
		// Traversal never leaves the frontend build: it falls back to index.html.
		{"traversal stays inside", "/../../etc/passwd", 200, "<title>mediaserver</title>", "no-cache"},
		{"encoded traversal stays inside", "/%2e%2e/%2e%2e/etc/passwd", 200, "<title>mediaserver</title>", "no-cache"},
		{"traversal to a file is 404", "/../../etc/hosts.txt", 404, "", ""},
		{"unknown api is JSON 404", "/api/nope", 404, `{"error":"not found"}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, header, body := get(t, srv, tt.path)
			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", status, tt.wantStatus, body)
			}
			if strings.Contains(body, "root:") {
				t.Fatalf("body leaks a file outside the build: %q", body)
			}
			if !strings.Contains(body, tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", body, tt.wantBody)
			}
			if tt.wantCache != "" && header.Get("Cache-Control") != tt.wantCache {
				t.Errorf("Cache-Control = %q, want %q", header.Get("Cache-Control"), tt.wantCache)
			}
		})
	}
}
