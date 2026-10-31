package scanner

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mediaserver/internal/db"
	"mediaserver/internal/probe"
	"mediaserver/migrations"
)

// fakeProber returns a fixed result and counts calls.
type fakeProber struct{ calls atomic.Int64 }

func (f *fakeProber) Probe(_ context.Context, file string) (*probe.Result, error) {
	f.calls.Add(1)
	if strings.Contains(file, "corrupt") {
		return nil, errors.New("invalid data found when processing input")
	}
	return &probe.Result{
		Container: "mkv", DurationMs: 1_440_000, Width: 1920, Height: 1080, VideoCodec: "h264", AudioCodec: "aac",
		Audio:     []probe.Track{{Index: 1, Codec: "aac", Language: "ja"}},
		Subtitles: []probe.Track{{Index: 2, Codec: "ass", Language: "pt", Default: true}},
		Raw:       []byte(`{}`),
	}, nil
}

type env struct {
	root   string
	db     *sql.DB
	sc     *Scanner
	prober *fakeProber
}

func newEnv(t testing.TB) *env {
	t.Helper()
	root := filepath.Join(t.TempDir(), "Mídias da Casa")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := db.Migrate(context.Background(), conn, migrations.FS); err != nil {
		t.Fatal(err)
	}
	fp := &fakeProber{}
	sc, err := New(context.Background(), conn, Options{Root: root, Workers: 4, Prober: fp})
	if err != nil {
		t.Fatal(err)
	}
	return &env{root: root, db: conn, sc: sc, prober: fp}
}

// write creates rel with unique content (its own path) so every file has a
// different partial hash.
func (e *env) write(t testing.TB, rel string) {
	t.Helper()
	p := filepath.Join(e.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("content of "+rel), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (e *env) scan(t testing.TB) Status {
	t.Helper()
	st, err := e.sc.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return st
}

func (e *env) count(t testing.TB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.db.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func (e *env) str(t testing.TB, query string, args ...any) string {
	t.Helper()
	var s sql.NullString
	if err := e.db.QueryRowContext(context.Background(), query, args...).Scan(&s); err != nil {
		t.Fatalf("%s %v: %v", query, args, err)
	}
	return s.String
}

func (e *env) fileID(t testing.TB, rel string) string {
	t.Helper()
	return e.str(t, `SELECT id FROM media_files WHERE rel_path = ?`, rel)
}

var sampleLibrary = []string{
	// movies
	"Filmes/Avatar (2009)/Avatar (2009).mkv",
	"Filmes/Avatar (2009)/poster.jpg",
	"Filmes/Avatar (2009)/fanart.jpg",
	"Filmes/Avatar (2009)/Avatar (2009).pt-BR.srt",
	"Filmes/Avatar (2009)/Avatar (2009).en.forced.srt",
	"Filmes/Avatar (2009)/Avatar-trailer.mkv",
	"Filmes/Avatar (2009)/Extras/Making Of.mkv",
	"Filmes/Avatar (2009)/Sample/avatar-sample.mkv",
	"The.Matrix.1999.1080p.BluRay.x264-SPARKS.mkv",
	"Cidade de Deus (2002).mkv",
	// series spread over folders and naming styles
	"Series/Dark/poster.jpg",
	"Series/Dark/Season 1/Dark.S01E01.Secrets.mkv",
	"Series/Dark/Season 1/Dark.S01E02.Lies.mkv",
	"Series/Dark/Season 1/Dark.S01E02.Lies.en.srt",
	"Series/Dark/Season 2/01.mkv",
	"Series/Dark/season02-poster.jpg",
	"Series/Dark/Trailers/Dark Season 3 Trailer.mkv",
	"Downloads/dark.s03e01.1080p.web.mkv",
	// anime: same show in two places, with a v2 release
	"[SubsPlease] Sousou no Frieren - 01 (1080p) [ABCD1234].mkv",
	"[SubsPlease] Sousou no Frieren - 01 (1080p) [ABCD1234].ass",
	"Animes/Sousou no Frieren/[SubsPlease] Sousou no Frieren - 02 (1080p).mkv",
	"Animes/Sousou no Frieren/[SubsPlease] Sousou no Frieren - 02v2 (1080p).mkv",
	"Animes/Sousou no Frieren/[SubsPlease] Sousou no Frieren - 03 (1080p).mkv",
	// review queue
	"Random Video.mkv",
	"Show - 05.mkv",
	// probe failure still indexed by name
	"Corrupt.Movie.2015.corrupt.mkv",
	// hidden and non-media files are ignored
	".hidden/Secret.2020.mkv",
	"notes.txt",
}

func TestScanSampleLibrary(t *testing.T) {
	e := newEnv(t)
	for _, rel := range sampleLibrary {
		e.write(t, rel)
	}
	st := e.scan(t)

	if st.Seen != 18 || st.Added != 18 {
		t.Fatalf("seen=%d added=%d, want 18/18", st.Seen, st.Added)
	}
	if st.ProbeErrs != 1 {
		t.Errorf("probe errors = %d, want 1", st.ProbeErrs)
	}

	checks := []struct {
		name  string
		query string
		args  []any
		want  string
	}{
		{"movies", `SELECT count(*) FROM movies`, nil, "4"},
		{"avatar poster", `SELECT poster_path FROM movies WHERE title = 'Avatar'`, nil, "Filmes/Avatar (2009)/poster.jpg"},
		{"avatar fanart", `SELECT fanart_path FROM movies WHERE title = 'Avatar'`, nil, "Filmes/Avatar (2009)/fanart.jpg"},
		{"avatar year", `SELECT year FROM movies WHERE title = 'Avatar'`, nil, "2009"},
		{"matrix", `SELECT year FROM movies WHERE title = 'The Matrix'`, nil, "1999"},
		{"accented title", `SELECT count(*) FROM movies WHERE title = 'Cidade de Deus'`, nil, "1"},
		{"corrupt still indexed", `SELECT count(*) FROM movies WHERE year = 2015`, nil, "1"},

		{"series count", `SELECT count(*) FROM series`, nil, "2"},
		{"dark grouped across folders", `SELECT count(*) FROM episodes e JOIN series s ON s.id = e.series_id WHERE s.title = 'Dark'`, nil, "4"},
		{"dark kind", `SELECT kind FROM series WHERE title = 'Dark'`, nil, "series"},
		{"dark seasons", `SELECT count(*) FROM seasons s JOIN series r ON r.id = s.series_id WHERE r.title = 'Dark'`, nil, "3"},
		{"dark poster", `SELECT poster_path FROM series WHERE title = 'Dark'`, nil, "Series/Dark/poster.jpg"},
		{"season 2 poster", `SELECT s.poster_path FROM seasons s JOIN series r ON r.id = s.series_id WHERE r.title = 'Dark' AND s.number = 2`, nil, "Series/Dark/season02-poster.jpg"},
		{"episode title", `SELECT e.title FROM episodes e JOIN seasons s ON s.id = e.season_id JOIN series r ON r.id = e.series_id WHERE r.title = 'Dark' AND s.number = 1 AND e.number = 2`, nil, "Lies"},

		{"frieren is anime", `SELECT kind FROM series WHERE norm_title = 'sousou no frieren'`, nil, "anime"},
		{"frieren episodes (v2 wins)", `SELECT count(*) FROM episodes e JOIN series s ON s.id = e.series_id WHERE s.norm_title = 'sousou no frieren'`, nil, "3"},
		{"v2 kept", `SELECT e.version FROM episodes e WHERE e.number = 2 AND e.series_id = (SELECT id FROM series WHERE norm_title = 'sousou no frieren')`, nil, "2"},
		{"v1 superseded", `SELECT status FROM media_files WHERE rel_path LIKE '%Frieren - 02 (1080p).mkv'`, nil, "ignored"},

		{"trailer linked to movie", `SELECT count(*) FROM extras x JOIN movies m ON m.id = x.owner_id WHERE x.extra_type = 'trailer' AND m.title = 'Avatar'`, nil, "1"},
		{"extra linked to movie", `SELECT count(*) FROM extras x JOIN movies m ON m.id = x.owner_id WHERE x.extra_type = 'other' AND x.title = 'Making Of'`, nil, "1"},
		{"series trailer", `SELECT count(*) FROM extras x JOIN series s ON s.id = x.owner_id WHERE x.owner_type = 'series' AND s.title = 'Dark'`, nil, "1"},
		{"sample ignored", `SELECT status FROM media_files WHERE rel_path LIKE '%avatar-sample.mkv'`, nil, "ignored"},

		{"review queue", `SELECT count(*) FROM media_files WHERE status = 'review'`, nil, "2"},
		{"review reason", `SELECT review_reason FROM media_files WHERE rel_path = 'Random Video.mkv'`, nil, "no_year"},
		{"review guess", `SELECT json_extract(guess_json, '$.kind') FROM media_files WHERE rel_path = 'Show - 05.mkv'`, nil, "series"},

		{"external subs avatar", `SELECT count(*) FROM subtitles s JOIN movies m ON m.media_file_id = s.media_file_id WHERE s.source = 'external' AND m.title = 'Avatar'`, nil, "2"},
		{"sub language", `SELECT language FROM subtitles WHERE rel_path LIKE '%pt-BR.srt'`, nil, "pt-BR"},
		{"sub forced", `SELECT is_forced FROM subtitles WHERE rel_path LIKE '%en.forced.srt'`, nil, "1"},
		{"anime ass sidecar", `SELECT codec FROM subtitles WHERE rel_path LIKE '%.ass'`, nil, "ass"},
		{"embedded subs (corrupt file has none)", `SELECT count(*) FROM subtitles WHERE source = 'embedded'`, nil, "17"},
		{"probe data", `SELECT video_codec FROM media_files WHERE rel_path = 'Cidade de Deus (2002).mkv'`, nil, "h264"},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if got := e.str(t, c.query, c.args...); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestIncrementalScan(t *testing.T) {
	e := newEnv(t)
	for _, rel := range sampleLibrary {
		e.write(t, rel)
	}
	e.scan(t)
	calls := e.prober.calls.Load()
	matrixID := e.fileID(t, "The.Matrix.1999.1080p.BluRay.x264-SPARKS.mkv")
	movieID := e.str(t, `SELECT id FROM movies WHERE media_file_id = ?`, matrixID)
	darkID := e.str(t, `SELECT id FROM series WHERE title = 'Dark'`)

	// No changes: nothing probed, everything unchanged, IDs stable.
	st := e.scan(t)
	if st.Unchanged != st.Seen || st.Added+st.Updated+st.Moved+st.Removed != 0 {
		t.Fatalf("second scan: %+v", st)
	}
	if e.prober.calls.Load() != calls {
		t.Fatal("unchanged files were probed again")
	}

	// Changed in place (new size): re-probed, same ID.
	p := filepath.Join(e.root, "Cidade de Deus (2002).mkv")
	if err := os.WriteFile(p, []byte("a different, longer content"), 0o600); err != nil {
		t.Fatal(err)
	}
	cdd := e.fileID(t, "Cidade de Deus (2002).mkv")
	st = e.scan(t)
	if st.Updated != 1 || e.fileID(t, "Cidade de Deus (2002).mkv") != cdd {
		t.Fatalf("update: %+v", st)
	}

	// Renamed + moved: same media_file and movie IDs.
	newRel := "Filmes/Matrix/The Matrix (1999).mkv"
	if err := os.MkdirAll(filepath.Join(e.root, "Filmes", "Matrix"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(e.root, "The.Matrix.1999.1080p.BluRay.x264-SPARKS.mkv"),
		filepath.Join(e.root, filepath.FromSlash(newRel))); err != nil {
		t.Fatal(err)
	}
	st = e.scan(t)
	if st.Moved != 1 || st.Removed != 0 || st.Added != 0 {
		t.Fatalf("move: %+v", st)
	}
	if got := e.fileID(t, newRel); got != matrixID {
		t.Fatalf("moved file id %s, want %s", got, matrixID)
	}
	if got := e.str(t, `SELECT id FROM movies WHERE media_file_id = ?`, matrixID); got != movieID {
		t.Fatalf("movie id changed: %s -> %s", movieID, got)
	}
	if got := e.str(t, `SELECT id FROM series WHERE title = 'Dark'`); got != darkID {
		t.Fatalf("series id changed: %s -> %s", darkID, got)
	}

	// Deleted from disk: removed from the database.
	if err := os.Remove(filepath.Join(e.root, "Random Video.mkv")); err != nil {
		t.Fatal(err)
	}
	st = e.scan(t)
	if st.Removed != 1 || e.count(t, `SELECT count(*) FROM media_files WHERE rel_path = 'Random Video.mkv'`) != 0 {
		t.Fatalf("remove: %+v", st)
	}

	// A new subtitle alone triggers a rebuild.
	e.write(t, "Cidade de Deus (2002).pt-BR.srt")
	e.scan(t)
	if n := e.count(t, `SELECT count(*) FROM subtitles WHERE rel_path = 'Cidade de Deus (2002).pt-BR.srt'`); n != 1 {
		t.Fatalf("new sidecar subtitle not linked (%d)", n)
	}
}

func TestPendingFilesWaitForCopy(t *testing.T) {
	e := newEnv(t)
	e.sc.opt.MinAge = time.Hour // everything looks "still copying"
	e.write(t, "The.Matrix.1999.mkv")
	st := e.scan(t)
	if st.Pending != 1 || st.Added != 0 {
		t.Fatalf("pending file was indexed: %+v", st)
	}
	e.sc.opt.MinAge = 0
	if st := e.scan(t); st.Added != 1 {
		t.Fatalf("file not indexed after copy finished: %+v", st)
	}
}

// snapshot records mtime + SHA-256 of every file under root.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[p] = info.ModTime().String() + " " + hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUserFilesAreNeverModified(t *testing.T) {
	e := newEnv(t)
	for _, rel := range sampleLibrary {
		e.write(t, rel)
	}
	before := snapshot(t, e.root)
	e.scan(t)
	e.scan(t)
	if err := e.sc.Rematerialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, e.root)
	if len(before) != len(after) {
		t.Fatalf("file count changed: %d -> %d", len(before), len(after))
	}
	for p, b := range before {
		if after[p] != b {
			t.Errorf("%s changed: %s -> %s", p, b, after[p])
		}
	}
}
