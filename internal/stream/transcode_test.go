package stream

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"mediaserver/internal/library"
	"mediaserver/internal/probe"
)

const testMediaID = "0b7e3c1a-5d6f-4a8b-9c0d-1e2f3a4b5c6d"

// newTranscoder makes a MEDIA_DIR (with spaces and accents) holding a
// generated MKV of seconds s: H.264 with a keyframe every second + AC-3.
func newTranscoder(t *testing.T, seconds, maxTranscodes int) (*Transcoder, library.Media, string) {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not in PATH")
	}
	media := filepath.Join(t.TempDir(), "mídia 日本")
	if err := os.MkdirAll(media, 0o750); err != nil {
		t.Fatal(err)
	}
	rel := "Filme (2020).mkv"
	d := strconv.Itoa(seconds)
	out, err := exec.CommandContext(t.Context(), ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x180:rate=24:duration="+d,
		"-f", "lavfi", "-i", "sine=frequency=440:duration="+d,
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "24", "-pix_fmt", "yuv420p", "-c:a", "ac3",
		filepath.Join(media, rel)).CombinedOutput()
	if err != nil {
		t.Fatalf("make test file: %v %s", err, out)
	}
	root, err := os.OpenRoot(media)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	tr, err := NewTranscoder(t.Context(), TranscoderOptions{
		Root: root, FFmpeg: ffmpeg, FFprobe: ffprobe, Dir: filepath.Join(t.TempDir(), "transcode"),
		HWAccel: "none", MaxTranscodes: maxTranscodes, CacheBytes: 1 << 30,
		Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tr.wg.Wait) // no FFmpeg outlives a test
	m := library.Media{
		ID: testMediaID, Container: "mkv", VideoCodec: "h264", VideoBits: 8, DurationMs: int64(seconds) * 1000,
		Audio: []probe.Track{{Index: 1, Codec: "ac3", Default: true}},
	}
	return tr, m, rel
}

func serve(t *testing.T, tr *Transcoder, url string) (int, string) {
	t.Helper()
	parts := strings.Split(url, "/") // /api/transcode/<key>/<file>
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err := tr.ServeFile(rec, req, parts[3], parts[4]); err != nil {
		if errors.Is(err, ErrNotFound) {
			return http.StatusNotFound, ""
		}
		t.Fatalf("serve %s: %v", url, err)
	}
	return rec.Code, rec.Body.String()
}

func running(tr *Transcoder) int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return len(tr.sessions)
}

func waitIdle(t *testing.T, tr *Transcoder) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for running(tr) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("FFmpeg still running")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestTranscodeSession(t *testing.T) {
	tr, m, rel := newTranscoder(t, 6, 1)
	// The client plays H.264 + AAC but neither MKV nor AC-3: remux + AAC.
	req := PlayRequest{Caps: []string{"mp4", "h264", "aac"}, Audio: 1}
	pb, err := tr.Start(t.Context(), m, rel, req)
	if err != nil {
		t.Fatal(err)
	}
	if pb.Direct || pb.Video != "copy" || pb.Audio != "aac" || pb.Start != 0 ||
		!strings.HasPrefix(pb.URL, "/api/transcode/") || !strings.HasSuffix(pb.URL, "/index.m3u8") {
		t.Fatalf("playback = %+v", pb)
	}
	code, body := serve(t, tr, pb.URL) // waits for the first segment
	if code != http.StatusOK || !strings.Contains(body, "#EXTINF") || !strings.Contains(body, "init.mp4") {
		t.Fatalf("playlist = %d %q", code, body)
	}
	waitIdle(t, tr)
	_, body = serve(t, tr, pb.URL)
	if !strings.Contains(body, "#EXT-X-ENDLIST") {
		t.Fatalf("finished playlist = %q", body)
	}
	base := strings.TrimSuffix(pb.URL, "index.m3u8")
	for _, f := range []string{"init.mp4", "seg0.m4s"} {
		if code, data := serve(t, tr, base+f); code != http.StatusOK || len(data) == 0 {
			t.Fatalf("%s = %d (%d bytes)", f, code, len(data))
		}
	}
	for _, f := range []string{"seg99.m4s", "../index.m3u8", "x.txt"} {
		if code, _ := serve(t, tr, base+f); code != http.StatusNotFound {
			t.Fatalf("%s = %d, want 404", f, code)
		}
	}

	// Same request: the finished session is the cache, no new FFmpeg.
	again, err := tr.Start(t.Context(), m, rel, req)
	if err != nil || again != pb || running(tr) != 0 {
		t.Fatalf("reuse = %+v %v (running %d)", again, err, running(tr))
	}

	// A video copy starts on a keyframe (one per second here).
	req.Start = 3.6
	at, err := tr.Start(t.Context(), m, rel, req)
	if err != nil || at.Start != 3 || at.URL == pb.URL {
		t.Fatalf("seek = %+v %v", at, err)
	}
	waitIdle(t, tr)

	// The player supports everything: direct play, nothing to run.
	direct, err := tr.Start(t.Context(), m, rel, PlayRequest{Caps: []string{"mkv", "h264", "ac3"}, Start: 2})
	if err != nil || !direct.Direct || direct.URL != "/api/media/"+testMediaID+"/stream" || direct.Start != 2 {
		t.Fatalf("direct = %+v %v", direct, err)
	}
}

func TestTranscodeStop(t *testing.T) {
	tr, m, rel := newTranscoder(t, 600, 1)
	encode := PlayRequest{Caps: nil, Audio: 1} // nothing supported: H.264 + AAC encode

	pb, err := tr.Start(t.Context(), m, rel, encode)
	if err != nil || pb.Video != "h264" || pb.Audio != "aac" {
		t.Fatalf("start = %+v %v", pb, err)
	}
	// One video encode at a time (MAX_TRANSCODES=1); remuxes are not limited.
	if _, err := tr.Start(t.Context(), m, rel, PlayRequest{Audio: 1, Start: 10}); !errors.Is(err, ErrBusy) {
		t.Fatalf("second encode: %v, want ErrBusy", err)
	}
	remux, err := tr.Start(t.Context(), m, rel, PlayRequest{Caps: []string{"h264"}, Audio: 1})
	if err != nil || remux.Video != "copy" {
		t.Fatalf("remux = %+v %v", remux, err)
	}
	key := strings.Split(pb.URL, "/")[3]
	tr.Stop(key)
	if _, err := os.Stat(filepath.Join(tr.dir, key)); !os.IsNotExist(err) {
		t.Fatalf("stopped session output kept: %v", err)
	}
	if code, _ := serve(t, tr, pb.URL); code != http.StatusNotFound {
		t.Fatalf("stopped playlist = %d, want 404", code)
	}

	// Idle sessions (closed tab) are stopped by the reaper.
	tr.reap(time.Now().Add(time.Minute))
	waitIdle(t, tr)

	// 50 starts/stops in a row leave no FFmpeg behind (Sprint 5 acceptance).
	for i := range 50 {
		encode.Start = float64(i)
		pb, err := tr.Start(t.Context(), m, rel, encode)
		if err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
		tr.Stop(strings.Split(pb.URL, "/")[3])
	}
	if n := running(tr); n != 0 {
		t.Fatalf("%d sessions still running", n)
	}
	tr.wg.Wait()
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	tr := &Transcoder{dir: dir, cache: 250, sessions: map[string]*session{}}
	write := func(key, playlist string, size int, age time.Duration) {
		t.Helper()
		d := filepath.Join(dir, key)
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, playlistName), []byte(playlist), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "seg0.m4s"), bytes.Repeat([]byte{1}, size), 0o600); err != nil {
			t.Fatal(err)
		}
		at := time.Now().Add(-age)
		if err := os.Chtimes(d, at, at); err != nil {
			t.Fatal(err)
		}
	}
	done := "#EXTM3U\n#EXT-X-ENDLIST\n"
	write("old", done, 100, 3*time.Hour)
	write("mid", done, 100, 2*time.Hour)
	write("new", done, 100, time.Hour)
	write("partial", "#EXTM3U\n", 10, 0)        // stopped session leftover
	write("live", "#EXTM3U\n", 10, 4*time.Hour) // running: never touched
	tr.sessions["live"] = &session{}

	tr.prune()
	for key, want := range map[string]bool{"old": false, "mid": true, "new": true, "partial": false, "live": true} {
		if _, err := os.Stat(filepath.Join(dir, key)); (err == nil) != want {
			t.Errorf("%s kept = %v, want %v", key, err == nil, want)
		}
	}
}
