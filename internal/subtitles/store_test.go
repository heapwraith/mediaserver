package subtitles

import (
	"bytes"
	"database/sql"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mediaserver/internal/db"
	"mediaserver/internal/probe"
	"mediaserver/internal/scanner"
	"mediaserver/migrations"
)

const assFile = `[Script Info]
ScriptType: v4.00+
PlayResX: 640
PlayResY: 360

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Arial,28,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,0,2,10,10,10,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.00,0:00:04.00,Default,,0,0,0,,{\pos(320,60)}{\k50}Ka{\k50}ra{\k50}o{\k50}kê
`

// newStore scans a MEDIA_DIR holding an MKV with an embedded SubRip, an
// embedded ASS and an attached font (named to escape its folder), plus a
// Latin-1 sidecar SRT.
func newStore(t *testing.T) (*Store, *sql.DB, string) {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not in PATH")
	}
	tmp := t.TempDir()
	media := filepath.Join(tmp, "mídia")
	src := filepath.Join(tmp, "src")
	for _, d := range []string{media, src} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(src, "pt.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nAção embutida\n\n2\n00:00:20,000 --> 00:00:21,000\nfim\n"))
	write(filepath.Join(src, "k.ass"), []byte(assFile))
	write(filepath.Join(src, "font.ttf"), []byte("not really a font"))
	out, err := exec.CommandContext(t.Context(), ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x180:rate=24:duration=5",
		"-i", filepath.Join(src, "pt.srt"), "-i", filepath.Join(src, "k.ass"),
		"-map", "0", "-map", "1", "-map", "2", "-c:v", "libx264", "-preset", "ultrafast", "-c:s:0", "srt", "-c:s:1", "ass",
		"-metadata:s:s:0", "language=por", "-metadata:s:s:1", "language=por",
		"-attach", filepath.Join(src, "font.ttf"), "-metadata:s:t:0", "mimetype=application/x-truetype-font",
		"-metadata:s:t:0", "filename=../../evil.ttf",
		filepath.Join(media, "Filme (2020).mkv")).CombinedOutput()
	if err != nil {
		t.Fatalf("make test file: %v %s", err, out)
	}
	// "ação, você" in Latin-1.
	write(filepath.Join(media, "Filme (2020).pt-BR.srt"),
		[]byte("1\r\n00:00:02,000 --> 00:00:04,000\r\na\xe7\xe3o, voc\xea\r\n"))

	conn, err := db.Open(t.Context(), filepath.Join(tmp, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := db.Migrate(t.Context(), conn, migrations.FS); err != nil {
		t.Fatal(err)
	}
	sc, err := scanner.New(t.Context(), conn, scanner.Options{Root: media, Prober: probe.Prober{Path: ffprobe}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sc.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(media)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	s, err := NewStore(conn, root, ffmpeg, filepath.Join(tmp, "cache"), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return s, conn, tmp
}

func subID(t *testing.T, conn *sql.DB, where string) (id, mediaID string) {
	t.Helper()
	if err := conn.QueryRowContext(t.Context(), `SELECT id, media_file_id FROM subtitles WHERE `+where).Scan(&id, &mediaID); err != nil {
		t.Fatalf("subtitle %s: %v", where, err)
	}
	return id, mediaID
}

func TestStore(t *testing.T) {
	s, conn, tmp := newStore(t)
	ctx := t.Context()
	srt, mediaID := subID(t, conn, `source = 'embedded' AND codec = 'subrip'`)
	ass, _ := subID(t, conn, `source = 'embedded' AND codec = 'ass'`)
	ext, _ := subID(t, conn, `source = 'external'`)
	// Simultaneous players must receive the same completed extraction.
	type prepared struct {
		dir string
		err error
	}
	results := make(chan prepared, 8)
	for range 8 {
		go func() {
			dir, err := s.prepare(ctx, mediaID)
			results <- prepared{dir, err}
		}()
	}
	var cache string
	for range 8 {
		result := <-results
		if result.err != nil || (cache != "" && result.dir != cache) {
			t.Fatalf("concurrent preparation = %+v, want %s", result, cache)
		}
		cache = result.dir
	}

	vtt, err := s.VTT(ctx, srt, 0)
	if err != nil || !strings.Contains(string(vtt), "00:00:01.000 --> 00:00:03.000\nAção embutida") {
		t.Fatalf("embedded vtt = %q %v", vtt, err)
	}
	// An HLS session starting at 0:10: only the last cue, 10 s earlier.
	vtt, err = s.VTT(ctx, srt, 10e9)
	if err != nil || strings.Contains(string(vtt), "Ação") || !strings.Contains(string(vtt), "00:00:10.000 --> 00:00:11.000\nfim") {
		t.Fatalf("shifted vtt = %q %v", vtt, err)
	}
	vtt, err = s.VTT(ctx, ext, 0)
	if err != nil || !strings.Contains(string(vtt), "ação, você") {
		t.Fatalf("latin-1 sidecar = %q %v", vtt, err)
	}
	if _, err := s.VTT(ctx, ass, 0); err == nil {
		t.Fatal("ASS as VTT: want an error")
	}

	text, err := s.ASS(ctx, ass)
	if err != nil || !strings.Contains(string(text), `{\pos(320,60)}{\k50}Ka`) || !strings.Contains(string(text), "kê") {
		t.Fatalf("ass = %q %v", text, err)
	}

	fonts, err := s.Fonts(ctx, mediaID)
	if err != nil || len(fonts) != 1 || !strings.HasSuffix(fonts[0], ".ttf") {
		t.Fatalf("fonts = %v %v", fonts, err)
	}
	p, err := s.FontPath(ctx, mediaID, fonts[0])
	if data, _ := os.ReadFile(p); err != nil || !bytes.Equal(data, []byte("not really a font")) {
		t.Fatalf("font %s = %q %v", p, data, err)
	}
	if _, err := s.FontPath(ctx, mediaID, "../t.db"); err == nil {
		t.Fatal("font path traversal accepted")
	}
	// The attachment name stored in the file is never used as a path.
	for _, bad := range []string{filepath.Join(tmp, "evil.ttf"), filepath.Join(tmp, "cache", "evil.ttf")} {
		if _, err := os.Stat(bad); err == nil {
			t.Fatalf("attachment escaped to %s", bad)
		}
	}

	dir, file, err := s.BurnFile(ctx, ass)
	if err != nil || file == "" {
		t.Fatalf("burn file = %s %s %v", dir, file, err)
	}
	if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "fonts")); err != nil {
		t.Fatal(err)
	}

	// Extracted once: the cache is reused.
	done := filepath.Join(dir, doneMarker)
	before, _ := os.Stat(done)
	if _, err := s.VTT(ctx, srt, 0); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.Stat(done); !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("extracted again")
	}
	// A new file version must not delete a cache still being read by a player.
	changed := time.Now().Add(time.Second)
	if err := os.Chtimes(filepath.Join(s.rootDir, "Filme (2020).mkv"), changed, changed); err != nil {
		t.Fatal(err)
	}
	newDir, err := s.prepare(ctx, mediaID)
	if err != nil || newDir == dir {
		t.Fatalf("new version = %s %v", newDir, err)
	}
	if _, err := os.Stat(done); err != nil {
		t.Fatalf("previous cache removed: %v", err)
	}
}
