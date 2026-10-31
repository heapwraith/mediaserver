package api

import (
	"encoding/json/v2"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mediaserver/internal/library"
)

func decode[T any](t *testing.T, body string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return v
}

var libraryFiles = []string{
	"Filmes/The Matrix (1999)/The Matrix (1999).mkv",
	"Filmes/Alien (1979)/Alien (1979).mkv",
	"Filmes/Alien (1979)/Alien (1979)-trailer.mkv",
	"Series/Dark/Season 1/Dark - S01E01.mkv",
	"Dark.S01E02.1080p.WEB-DL.mkv",
	"Animes/[SubsPlease] Sousou no Frieren - 01 (1080p) [ABCD1234].mkv",
}

func TestLibraryEndpoints(t *testing.T) {
	srv, sc, fileID := newLibraryServer(t, libraryFiles...)
	if _, err := sc.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}

	status, hdr, body := get(t, srv, "/api/movies?sort=year")
	movies := decode[library.List[library.Movie]](t, body)
	if status != http.StatusOK || movies.Total != 2 || len(movies.Items) != 2 || movies.Items[0].Title != "Alien" {
		t.Fatalf("movies = %d %s", status, body)
	}
	alien := movies.Items[0]
	if alien.MediaID != fileID("Filmes/Alien (1979)/Alien (1979).mkv") || alien.Year != 1979 {
		t.Fatalf("alien = %+v", alien)
	}

	// ETag revalidation.
	status2, _, _ := get(t, srv, "/api/movies?sort=year", "If-None-Match: "+hdr.Get("ETag"))
	if hdr.Get("ETag") == "" || status2 != http.StatusNotModified {
		t.Fatalf("etag %q, revalidation = %d", hdr.Get("ETag"), status2)
	}

	_, _, body = get(t, srv, "/api/movies?sort=title&order=desc&limit=1")
	page := decode[library.List[library.Movie]](t, body)
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].Title != "The Matrix" || page.Limit != 1 {
		t.Fatalf("page = %s", body)
	}

	// Local search ignores case, accents and punctuation.
	_, _, body = get(t, srv, "/api/movies?q=M%C3%81TRIX!")
	if found := decode[library.List[library.Movie]](t, body); found.Total != 1 || found.Items[0].Title != "The Matrix" {
		t.Fatalf("search = %s", body)
	}
	_, _, body = get(t, srv, "/api/animes?q=frieren")
	if found := decode[library.List[library.Series]](t, body); found.Total != 1 {
		t.Fatalf("anime search = %s", body)
	}

	_, _, body = get(t, srv, "/api/series")
	series := decode[library.List[library.Series]](t, body)
	if series.Total != 1 || series.Items[0].Title != "Dark" || series.Items[0].Episodes != 2 {
		t.Fatalf("series = %s", body)
	}
	_, _, body = get(t, srv, "/api/animes")
	if animes := decode[library.List[library.Series]](t, body); animes.Total != 1 || animes.Items[0].Kind != "anime" {
		t.Fatalf("animes = %s", body)
	}

	_, _, body = get(t, srv, "/api/series/"+series.Items[0].ID)
	dark := decode[library.SeriesDetail](t, body)
	if len(dark.Seasons) != 1 || dark.Seasons[0].Number != 1 || len(dark.Seasons[0].Episodes) != 2 ||
		dark.Seasons[0].Episodes[1].Number != 2 {
		t.Fatalf("series detail = %s", body)
	}

	_, _, body = get(t, srv, "/api/movies/"+alien.ID)
	if m := decode[library.Movie](t, body); m != alien {
		t.Fatalf("movie = %s", body)
	}

	_, _, body = get(t, srv, "/api/movies/"+alien.ID+"/extras")
	if extras := decode[[]library.Extra](t, body); len(extras) != 1 || extras[0].Type != "trailer" ||
		extras[0].MediaID != fileID("Filmes/Alien (1979)/Alien (1979)-trailer.mkv") {
		t.Fatalf("extras = %s", body)
	}
	_, _, body = get(t, srv, "/api/series/"+series.Items[0].ID+"/extras")
	if body != "[]" {
		t.Fatalf("series extras = %s", body)
	}

	_, _, body = get(t, srv, "/api/media/"+alien.MediaID)
	if m := decode[library.Media](t, body); m.FileName != "Alien (1979).mkv" || m.Kind != "movie" {
		t.Fatalf("media = %s", body)
	}

	const unknown = "0190f3c0-0000-7000-8000-000000000000"
	errs := []struct {
		path string
		want int
	}{
		{"/api/movies?sort=rating", 400},
		{"/api/movies?order=up", 400},
		{"/api/movies?limit=0", 400},
		{"/api/movies?limit=501", 400},
		{"/api/movies?offset=-1", 400},
		{"/api/animes?limit=abc", 400},
		{"/api/series/" + unknown, 404},
		{"/api/series/123", 400},
		{"/api/movies/" + unknown, 404},
		{"/api/movies/" + unknown + "/extras", 404},
		{"/api/series/" + unknown + "/extras", 404},
		{"/api/media/" + unknown, 404},
	}
	for _, e := range errs {
		if status, _, body := get(t, srv, e.path); status != e.want {
			t.Errorf("GET %s = %d %s, want %d", e.path, status, body, e.want)
		}
	}
}

func TestStreamEndpoint(t *testing.T) {
	srv, sc, fileID := newLibraryServer(t, libraryFiles...)
	if _, err := sc.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	rel := "Filmes/The Matrix (1999)/The Matrix (1999).mkv"
	id := fileID(rel)

	status, hdr, body := get(t, srv, "/api/media/"+id+"/stream", "Range: bytes=7-12")
	if status != http.StatusPartialContent || body != rel[7:13] ||
		hdr.Get("Content-Type") != "video/x-matroska" || hdr.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("range = %d %q %v", status, body, hdr)
	}
	status, hdr, body = get(t, srv, "/api/media/"+id+"/stream")
	if status != http.StatusOK || body != rel {
		t.Fatalf("full = %d %q", status, body)
	}
	// Conditional request: the browser revalidates instead of downloading again.
	if status2, _, _ := get(t, srv, "/api/media/"+id+"/stream", "If-None-Match: "+hdr.Get("ETag")); status2 != 304 {
		t.Fatalf("revalidation = %d", status2)
	}

	// Traversal in the URL never reaches the file system (the stored-path
	// case is covered in internal/stream).
	for _, p := range []string{
		"/api/media/..%2f..%2fsecret.mkv/stream",
		"/api/media/%2e%2e/stream",
		"/api/media/../../secret.mkv/stream",
	} {
		if status, _, body := get(t, srv, p); status != 404 && status != 400 {
			t.Errorf("GET %s = %d %q, want 400/404", p, status, body)
		}
	}

	// File deleted after the scan.
	trailer := "Filmes/Alien (1979)/Alien (1979)-trailer.mkv"
	if err := os.Remove(filepath.Join(sc.Root(), filepath.FromSlash(trailer))); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := get(t, srv, "/api/media/"+fileID(trailer)+"/stream"); status != 404 {
		t.Fatalf("deleted file = %d", status)
	}
}

func TestImageEndpoint(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	srv, sc, _ := newLibraryServer(t, "Broken (2001).mkv") // not a real video
	video := filepath.Join(sc.Root(), "Clip (2020).mkv")
	out, err := exec.CommandContext(t.Context(), ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=10:duration=2", "-c:v", "mpeg4", video).CombinedOutput()
	if err != nil {
		t.Fatalf("make test video: %v %s", err, out)
	}
	if _, err := sc.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, _, body := get(t, srv, "/api/movies")
	movies := decode[library.List[library.Movie]](t, body).Items
	if len(movies) != 2 || movies[1].Title != "Clip" {
		t.Fatalf("movies = %s", body)
	}
	broken, clip := movies[0], movies[1]

	for _, q := range []string{"?w=480", "?variant=backdrop&w=1920", "?variant=blur&w=960", ""} {
		status, hdr, body := get(t, srv, "/api/images/"+clip.ID+q)
		if status != http.StatusOK || hdr.Get("Content-Type") != "image/webp" ||
			!strings.HasPrefix(body, "RIFF") || body[8:12] != "WEBP" {
			t.Fatalf("image %s = %d %v", q, status, hdr)
		}
	}
	// The episode/media id works too, and the card color reaches the listing.
	if status, _, _ := get(t, srv, "/api/images/"+clip.MediaID); status != http.StatusOK {
		t.Fatalf("image by media id = %d", status)
	}
	_, _, body = get(t, srv, "/api/movies")
	if c := decode[library.List[library.Movie]](t, body).Items[1].Color; len(c) != 7 || c[0] != '#' {
		t.Fatalf("color = %q", c)
	}

	for _, e := range []struct {
		path string
		want int
	}{
		{"/api/images/" + broken.ID, 404}, // ffmpeg fails: card falls back to typography
		{"/api/images/" + broken.ID, 404}, // and is not retried
		{"/api/images/" + clip.ID + "?w=100", 400},
		{"/api/images/" + clip.ID + "?variant=poster", 400},
		{"/api/images/0190f3c0-0000-7000-8000-000000000000", 404},
	} {
		if status, _, body := get(t, srv, e.path); status != e.want {
			t.Errorf("GET %s = %d %s, want %d", e.path, status, body, e.want)
		}
	}
}
