package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mediaserver/internal/library"
	"mediaserver/internal/stream"
)

func TestPlayback(t *testing.T) {
	rel := "Filmes/Alien (1979)/Alien (1979).mkv"
	srv, sc, fileID, conn := newLibraryServerDB(t, rel)
	if _, err := sc.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	id := fileID(rel)
	probeJSON := `{"streams": [
		{"index": 0, "codec_type": "video", "codec_name": "h264", "pix_fmt": "yuv420p"},
		{"index": 1, "codec_type": "audio", "codec_name": "aac", "channels": 2, "disposition": {"default": 1}}],
		"format": {"format_name": "matroska,webm", "duration": "60"}}`
	if _, err := conn.ExecContext(t.Context(),
		`UPDATE media_files SET container = 'mkv', video_codec = 'h264', duration_ms = 60000, probe_json = ? WHERE id = ?`,
		probeJSON, id); err != nil {
		t.Fatal(err)
	}

	// A client that plays MKV + H.264 + AAC gets the file itself.
	status, body := do(t, srv, http.MethodPost, "/api/media/"+id+"/playback",
		`{"caps": ["mkv", "h264", "aac"], "audio": 1, "start": 12.5}`)
	pb := decode[stream.Playback](t, body)
	if status != http.StatusOK || !pb.Direct || pb.URL != "/api/media/"+id+"/stream" || pb.Start != 12.5 {
		t.Fatalf("playback = %d %s", status, body)
	}

	for _, c := range []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPost, "/api/media/" + id + "/playback", `{"caps": "mkv"}`, http.StatusBadRequest},
		{http.MethodPost, "/api/media/" + id + "/playback", `{"unknown": 1}`, http.StatusBadRequest},
		{http.MethodPost, "/api/media/0b7e3c1a-5d6f-4a8b-9c0d-1e2f3a4b5c6d/playback", `{}`, http.StatusNotFound},
		{http.MethodGet, "/api/transcode/../../index.m3u8", "", http.StatusNotFound},
		{http.MethodGet, "/api/transcode/0123456789abcdef0123456789abcdef/index.m3u8", "", http.StatusNotFound},
		{http.MethodDelete, "/api/transcode/0123456789abcdef0123456789abcdef", "", http.StatusNoContent},
	} {
		if status, body := do(t, srv, c.method, c.path, c.body); status != c.want {
			t.Errorf("%s %s = %d %s, want %d", c.method, c.path, status, body, c.want)
		}
	}
}

// The frontend turns these codes into friendly messages (web/src/lib/api/errors.ts).
func TestErrorCodes(t *testing.T) {
	rel := "Filmes/Alien (1979)/Alien (1979).mkv"
	srv, sc, fileID, _ := newLibraryServerDB(t, rel)
	if _, err := sc.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	id := fileID(rel)
	// The file goes away after the scan: the library still lists it.
	if err := os.Remove(filepath.Join(sc.Root(), filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		method, path, body string
		want               int
		code               string
	}{
		{http.MethodPost, "/api/media/" + id + "/playback", `{"caps": ["mkv", "h264", "aac"]}`, http.StatusGone, "file_missing"},
		{http.MethodPost, "/api/media/" + id + "/playback", `{"caps": []}`, http.StatusGone, "file_missing"},
		{http.MethodGet, "/api/media/" + id + "/stream", "", http.StatusGone, "file_missing"},
		{http.MethodGet, "/api/media/0b7e3c1a-5d6f-4a8b-9c0d-1e2f3a4b5c6d", "", http.StatusNotFound, "not_found"},
		{http.MethodGet, "/api/media/x", "", http.StatusBadRequest, "invalid"},
	} {
		status, body := do(t, srv, c.method, c.path, c.body)
		if status != c.want || !strings.Contains(body, `"code":"`+c.code+`"`) {
			t.Errorf("%s %s = %d %s, want %d %s", c.method, c.path, status, body, c.want, c.code)
		}
	}
}

func TestWriteErrorCodes(t *testing.T) {
	d := Deps{Logger: slog.New(slog.DiscardHandler)}
	for _, c := range []struct {
		err  error
		want int
		code string
	}{
		{fmt.Errorf("start ffmpeg: %w", exec.ErrNotFound), http.StatusServiceUnavailable, "no_ffmpeg"},
		{stream.ErrBusy, http.StatusServiceUnavailable, "busy"},
		{fmt.Errorf("x: %w", library.ErrConflict), http.StatusConflict, "conflict"},
		{errors.New("boom"), http.StatusInternalServerError, "internal"},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/x", nil)
		d.writeError(w, r, c.err)
		if w.Code != c.want || !strings.Contains(w.Body.String(), `"code":"`+c.code+`"`) {
			t.Errorf("%v = %d %s, want %d %s", c.err, w.Code, w.Body.String(), c.want, c.code)
		}
	}
}
