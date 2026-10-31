package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json/v2"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mediaserver/internal/db"
	"mediaserver/internal/library"
	"mediaserver/internal/scanner"
	"mediaserver/internal/stream"
	"mediaserver/internal/subtitles"
	"mediaserver/migrations"
)

func newLibraryServer(t *testing.T, files ...string) (*httptest.Server, *scanner.Scanner, func(string) string) {
	t.Helper()
	srv, sc, fileID, _ := newLibraryServerDB(t, files...)
	return srv, sc, fileID
}

// newLibraryServerDB is newLibraryServer that also returns the database.
func newLibraryServerDB(t *testing.T, files ...string) (*httptest.Server, *scanner.Scanner, func(string) string, *sql.DB) {
	t.Helper()
	root := t.TempDir()
	for _, rel := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(rel), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	conn, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := db.Migrate(t.Context(), conn, migrations.FS); err != nil {
		t.Fatal(err)
	}
	sc, err := scanner.New(t.Context(), conn, scanner.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mediaRoot, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mediaRoot.Close() })
	store := library.Store{DB: conn}
	images, err := stream.NewImages(conn, mediaRoot, "ffmpeg", t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	transcoder, err := stream.NewTranscoder(t.Context(), stream.TranscoderOptions{
		Root: mediaRoot, FFmpeg: "ffmpeg", FFprobe: "ffprobe", Dir: t.TempDir(), HWAccel: "none",
		MaxTranscodes: 1, CacheBytes: 1 << 30, Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	subs, err := subtitles.NewStore(conn, mediaRoot, "ffmpeg", t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewRouter(Deps{
		Logger: slog.New(slog.DiscardHandler), DB: conn, Version: "test",
		Scanner: sc, StartScan: func() bool { return sc.Start(ctx) },
		Library: store, Stream: &stream.Server{Root: mediaRoot, Files: store}, Images: images,
		Transcoder: transcoder, Subtitles: subs, Secret: []byte("test-secret-0123456789abcdef"),
	}))
	t.Cleanup(srv.Close)
	fileID := func(rel string) string {
		var id string
		if err := conn.QueryRowContext(t.Context(), `SELECT id FROM media_files WHERE rel_path = ?`, rel).Scan(&id); err != nil {
			t.Fatalf("file id %s: %v", rel, err)
		}
		return id
	}
	return srv, sc, fileID, conn
}

// do sends a request; headers are "Name: value" request headers.
func do(t *testing.T, srv *httptest.Server, method, path, body string, headers ...string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, r)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range headers {
		if k, v, ok := strings.Cut(h, ": "); ok {
			req.Header.Set(k, v)
		}
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(data)
}

func TestScanEndpoints(t *testing.T) {
	srv, _, _ := newLibraryServer(t, "The.Matrix.1999.mkv", "Random Video.mkv")

	status, body := do(t, srv, http.MethodPost, "/api/library/scan", "")
	if status != http.StatusAccepted || !strings.Contains(body, `"started":true`) {
		t.Fatalf("POST scan = %d %s", status, body)
	}
	var st scanner.Status
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, body = do(t, srv, http.MethodGet, "/api/library/scan", "")
		if err := json.Unmarshal([]byte(body), &st); err != nil {
			t.Fatal(err)
		}
		if !st.Running && !st.FinishedAt.IsZero() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("scan did not finish: %s", body)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if st.Added != 2 || st.Review != 1 {
		t.Fatalf("status = %+v", st)
	}

	status, body = do(t, srv, http.MethodGet, "/api/review", "")
	var queue []scanner.ReviewItem
	if err := json.Unmarshal([]byte(body), &queue); err != nil || status != 200 {
		t.Fatalf("GET review = %d %s (%v)", status, body, err)
	}
	if len(queue) != 1 || queue[0].RelPath != "Random Video.mkv" || queue[0].Reason != "no_year" {
		t.Fatalf("queue = %+v", queue)
	}
}

func TestReviewEndpoint(t *testing.T) {
	srv, sc, fileID := newLibraryServer(t, "Random Video.mkv", "Show - 05.mkv")
	if _, err := sc.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	id := fileID("Random Video.mkv")
	tests := []struct {
		name       string
		path, body string
		wantStatus int
		wantBody   string
	}{
		{"movie", "/api/review/" + id, `{"kind":"movie","title":"Férias","year":2020}`, 200, `"status":"ok"`},
		{"series", "/api/review/" + fileID("Show - 05.mkv"), `{"kind":"series","title":"Show","season":1,"episode":5}`, 200, `"episode_id":`},
		{"not a uuid", "/api/review/../../etc/passwd", `{"kind":"ignored"}`, 404, ""},
		{"bad uuid", "/api/review/123", `{"kind":"ignored"}`, 400, "UUID"},
		{"unknown file", "/api/review/0190f3c0-0000-7000-8000-000000000000", `{"kind":"ignored"}`, 404, "not found"},
		{"bad json", "/api/review/" + id, `{kind:`, 400, "invalid JSON"},
		{"unknown field", "/api/review/" + id, `{"kind":"movie","title":"X","path":"/etc"}`, 400, "invalid JSON"},
		{"invalid kind", "/api/review/" + id, `{"kind":"podcast","title":"X"}`, 400, "kind must be"},
		{"missing episode", "/api/review/" + id, `{"kind":"anime","title":"X","season":1}`, 400, "episode is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, srv, http.MethodPut, tt.path, tt.body)
			if status != tt.wantStatus || !strings.Contains(body, tt.wantBody) {
				t.Fatalf("got %d %s, want %d containing %q", status, body, tt.wantStatus, tt.wantBody)
			}
		})
	}
	// Oversized body.
	big := `{"kind":"movie","title":"` + string(bytes.Repeat([]byte("a"), 2<<20)) + `"}`
	if status, _ := do(t, srv, http.MethodPut, "/api/review/"+id, big); status != 400 {
		t.Fatalf("oversized body: %d", status)
	}
}

func TestSeriesMergeSplitEndpoints(t *testing.T) {
	srv, sc, fileID := newLibraryServer(t,
		"[A] Shingeki no Kyojin - 01.mkv", "[A] Shingeki no Kyojin - 02.mkv",
		"[B] Attack on Titan - 03.mkv")
	if _, err := sc.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	snkFile, aotFile := fileID("[A] Shingeki no Kyojin - 01.mkv"), fileID("[B] Attack on Titan - 03.mkv")
	seriesOf := func(file string) string {
		st, err := sc.FileState(t.Context(), file)
		if err != nil {
			t.Fatal(err)
		}
		return st.SeriesID
	}
	snk, aot := seriesOf(snkFile), seriesOf(aotFile)

	status, body := do(t, srv, http.MethodPost, "/api/series/merge", `{"source_ids":["`+aot+`"],"target_id":"`+snk+`"}`)
	if status != 200 || !strings.Contains(body, snk) || seriesOf(aotFile) != snk {
		t.Fatalf("merge = %d %s", status, body)
	}

	status, body = do(t, srv, http.MethodPost, "/api/series/split",
		`{"series_id":"`+snk+`","file_ids":["`+aotFile+`"],"title":"Attack on Titan"}`)
	if status != 200 || seriesOf(aotFile) == snk {
		t.Fatalf("split = %d %s", status, body)
	}

	bad := []struct{ path, body string }{
		{"/api/series/merge", `{"source_ids":["x"],"target_id":"` + snk + `"}`},
		{"/api/series/merge", `{"source_ids":[],"target_id":"` + snk + `"}`},
		{"/api/series/split", `{"series_id":"` + snk + `","file_ids":["` + snkFile + `"],"title":""}`},
		{"/api/series/split", `{"series_id":"` + snk + `"}`},
	}
	for _, b := range bad {
		if status, body := do(t, srv, http.MethodPost, b.path, b.body); status != 400 {
			t.Errorf("%s %s = %d %s, want 400", b.path, b.body, status, body)
		}
	}
}

func TestLibraryUnavailable(t *testing.T) {
	srv := newTestServer(t, true) // no scanner
	for _, p := range []string{"/api/library/scan", "/api/review"} {
		if status, _ := do(t, srv, http.MethodGet, p, ""); status != http.StatusServiceUnavailable {
			t.Errorf("%s = %d, want 503", p, status)
		}
	}
}
