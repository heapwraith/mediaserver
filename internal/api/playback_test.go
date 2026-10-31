package api

import (
	"net/http"
	"testing"

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
