package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mediaserver/internal/library"
)

func TestSubtitleContent(t *testing.T) {
	const movie = "Filme (2020).mp4"
	srv, sc, fileID, conn := newLibraryServerDB(t, movie)
	var root string
	if err := conn.QueryRowContext(t.Context(), `SELECT root_path FROM libraries`).Scan(&root); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"Filme (2020).pt-BR.srt": []byte("1\n00:00:01,000 --> 00:00:04,000\nAção, você\n"),
		"Filme (2020).en.srt":    append([]byte("1\n00:00:01,000 --> 00:00:04,000\n"), 'a', 0xe7, 0xe3, 'o', ',', ' ', 'v', 'o', 'c', 0xea),
		"Filme (2020).ja.ass":    []byte("[Script Info]\nScriptType: v4.00+\n; ação, você\n"),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sc.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, body := do(t, srv, http.MethodGet, "/api/media/"+fileID(movie), "")
	m := decode[library.Media](t, body)
	if len(m.Subtitles) != 3 {
		t.Fatalf("subtitles = %s", body)
	}
	for _, sub := range m.Subtitles {
		path, ctype, want := "/vtt?offset=2", "text/vtt; charset=utf-8", "00:00:00.000 --> 00:00:02.000"
		if sub.Kind == "ass" {
			path, ctype, want = "/ass", "text/plain; charset=utf-8", "[Script Info]"
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/subtitles/"+sub.ID+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if err != nil || res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != ctype || !strings.Contains(string(data), want) || !strings.Contains(strings.ToLower(string(data)), "ação, você") {
			t.Fatalf("subtitle %s = %d %s %q: %v", sub.Language, res.StatusCode, res.Header.Get("Content-Type"), data, err)
		}
	}
}

func TestSubtitleRoutes(t *testing.T) {
	srv, _, _ := newLibraryServer(t, libraryFiles...)
	unknown := "0b7e3c1a-5d6f-4a8b-9c0d-1e2f3a4b5c6d"
	for path, want := range map[string]int{
		"/api/subtitles/" + unknown + "/vtt":             http.StatusNotFound,
		"/api/subtitles/" + unknown + "/vtt?offset=-1":   http.StatusBadRequest,
		"/api/subtitles/" + unknown + "/vtt?offset=NaN":  http.StatusBadRequest,
		"/api/subtitles/" + unknown + "/vtt?offset=Inf":  http.StatusBadRequest,
		"/api/subtitles/" + unknown + "/vtt?offset=1e20": http.StatusBadRequest,
		"/api/subtitles/not-a-uuid/ass":                  http.StatusBadRequest,
		"/api/media/" + unknown + "/fonts":               http.StatusNotFound,
		"/api/media/" + unknown + "/fonts/..%2Ft.db":     http.StatusNotFound,
	} {
		if status, body := do(t, srv, http.MethodGet, path, ""); status != want {
			t.Errorf("GET %s = %d %s, want %d", path, status, body, want)
		}
	}
}
