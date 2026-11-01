package intro

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"mediaserver/internal/db"
	"mediaserver/internal/probe"
	"mediaserver/internal/scanner"
	"mediaserver/migrations"
)

// song is a melody (a note every 1/speed s, stepping `step` semitones) that
// chromaprint can follow; different steps and speeds sound different.
func song(step int, speed float64, seconds int) string {
	return fmt.Sprintf("aevalsrc=0.4*sin(2*PI*220*pow(2\\,mod(floor(t*%g)*%d\\,12)/12)*t):d=%d", speed, step, seconds)
}

// makeEpisode writes an audio-only episode: its own music, the opening (the
// same song in every episode) at introAt, more of its own music, then the
// same credits song. (Plain noise would not do: to chromaprint all noise
// sounds alike.)
func makeEpisode(t *testing.T, ffmpeg, path string, step, introAt int) {
	t.Helper()
	parts := []string{
		song(step, 3, introAt),
		song(7, 2, 40),
		song(step+2, 1.5, 400),
		song(5, 2, 50),
	}
	args := make([]string, 0, 4+4*len(parts)+5)
	args = append(args, "-hide_banner", "-loglevel", "error", "-y")
	for _, p := range parts {
		args = append(args, "-f", "lavfi", "-i", p+":s=22050")
	}
	args = append(args, "-filter_complex", "[0][1][2][3]concat=n=4:v=0:a=1", "-c:a", "flac", path)
	if out, err := exec.CommandContext(t.Context(), ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("make %s: %v %s", path, err, out)
	}
}

func markers(t *testing.T, conn *sql.DB) map[string][2]time.Duration {
	t.Helper()
	rows, err := conn.QueryContext(t.Context(), `
		SELECT substr(mf.rel_path, -10), m.kind, m.start_ms, m.end_ms
		FROM media_markers m JOIN media_files mf ON mf.id = m.media_file_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][2]time.Duration{}
	for rows.Next() {
		var name, kind string
		var start, end int64
		if err := rows.Scan(&name, &kind, &start, &end); err != nil {
			t.Fatal(err)
		}
		out[name+" "+kind] = [2]time.Duration{time.Duration(start) * time.Millisecond, time.Duration(end) * time.Millisecond}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAnalyzer(t *testing.T) {
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
	season := filepath.Join(media, "Show", "Season 1")
	if err := os.MkdirAll(season, 0o750); err != nil {
		t.Fatal(err)
	}
	starts := map[string]int{"S01E01.mkv": 60, "S01E02.mkv": 75, "S01E03.mkv": 90}
	steps := map[string]int{"S01E01.mkv": 1, "S01E02.mkv": 3, "S01E03.mkv": 11} // each episode its own music
	for name, at := range starts {
		makeEpisode(t, ffmpeg, filepath.Join(season, "Show - "+name), steps[name], at)
	}

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

	a := NewAnalyzer(conn, media, ffmpeg, slog.New(slog.DiscardHandler))
	if err := a.Analyze(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := markers(t, conn)
	near := func(name string, got, want time.Duration) {
		t.Helper()
		if d := got - want; d < -2*time.Second || d > 2*time.Second {
			t.Errorf("%s = %v, want %v ± 2 s", name, got, want)
		}
	}
	for name, at := range starts {
		intro, ok := got[name+" intro"]
		if !ok {
			t.Errorf("%s: no opening (markers %v)", name, got)
			continue
		}
		near(name+" opening start", intro[0], time.Duration(at)*time.Second)
		near(name+" opening end", intro[1], time.Duration(at+40)*time.Second)
		credits, ok := got[name+" credits"]
		if !ok {
			t.Errorf("%s: no credits (markers %v)", name, got)
			continue
		}
		near(name+" credits start", credits[0], time.Duration(at+440)*time.Second)
	}

	// Nothing changed: nothing is fingerprinted again (FFmpeg is not even run).
	a.ffmpeg = filepath.Join(tmp, "no-ffmpeg")
	if err := a.Analyze(t.Context()); err != nil {
		t.Fatal(err)
	}
	if again := markers(t, conn); len(again) != len(got) {
		t.Fatalf("markers after a no-op analysis = %v, want %v", again, got)
	}
}
