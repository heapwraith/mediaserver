// SPDX-License-Identifier: GPL-3.0-or-later

package stream

import (
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"mediaserver/internal/db"
	"mediaserver/migrations"
)

type imagesEnv struct {
	t    *testing.T
	db   *sql.DB
	root string
	dir  string
}

func newImagesEnv(t *testing.T) *imagesEnv {
	t.Helper()
	conn, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := db.Migrate(t.Context(), conn, migrations.FS); err != nil {
		t.Fatal(err)
	}
	e := &imagesEnv{t: t, db: conn, root: t.TempDir(), dir: t.TempDir()}
	e.exec(`INSERT INTO libraries (id, name, root_path, created_at) VALUES ('lib', 'lib', '/', 1)`)
	return e
}

func (e *imagesEnv) exec(q string, args ...any) {
	e.t.Helper()
	if _, err := e.db.ExecContext(e.t.Context(), q, args...); err != nil {
		e.t.Fatal(err)
	}
}

func (e *imagesEnv) images(ffmpeg string) *Images {
	e.t.Helper()
	root, err := os.OpenRoot(e.root)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = root.Close() })
	im, err := NewImages(e.db, root, ffmpeg, e.dir, slog.New(slog.DiscardHandler))
	if err != nil {
		e.t.Fatal(err)
	}
	return im
}

// movie adds a movie whose video is the file rel, with a duration of 1 s.
func (e *imagesEnv) movie(id, rel string) {
	e.t.Helper()
	e.exec(`INSERT INTO media_files (id, library_id, rel_path, size, mtime_ns, added_at, updated_at, duration_ms)
	        VALUES (?, 'lib', ?, 1, 1, 1, 1, 1000)`, "file-"+id, rel)
	e.exec(`INSERT INTO movies (id, media_file_id, title, sort_title, norm_title, year, added_at)
	        VALUES (?, ?, ?, ?, ?, 2020, 1)`, id, "file-"+id, id, id, id)
}

func (e *imagesEnv) cached(id string) {
	e.t.Helper()
	if err := os.MkdirAll(filepath.Join(e.dir, id, "oldkey"), 0o750); err != nil {
		e.t.Fatal(err)
	}
	e.exec(`INSERT INTO image_cache (owner_id, source_key, color, updated_at) VALUES (?, 'oldkey', '#000000', 1)`, id)
}

func (e *imagesEnv) colors(id string) int {
	e.t.Helper()
	var n int
	if err := e.db.QueryRowContext(e.t.Context(), `SELECT count(*) FROM image_cache WHERE owner_id = ?`, id).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestSweepCleansWhatLeftTheLibrary(t *testing.T) {
	e := newImagesEnv(t)
	e.movie("kept", "kept.mkv")
	e.cached("kept")
	e.cached("gone")
	note := filepath.Join(e.dir, "note.txt") // only owners' folders are touched
	if err := os.WriteFile(note, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := e.images("no-such-ffmpeg").clean(t.Context()); err != nil {
		t.Fatal(err)
	}

	if !exists(filepath.Join(e.dir, "kept", "oldkey")) || e.colors("kept") != 1 {
		t.Error("the images of a movie in the library were removed")
	}
	if exists(filepath.Join(e.dir, "gone")) || e.colors("gone") != 0 {
		t.Error("the images of what left the library are still there")
	}
	if !exists(note) {
		t.Error("removed a file that is not an owner's folder")
	}
}

func TestSweepWithoutCacheFolder(t *testing.T) {
	e := newImagesEnv(t)
	e.dir = filepath.Join(e.dir, "not-yet")
	if err := e.images("no-such-ffmpeg").Sweep(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestSweepStopsWithoutFFmpeg(t *testing.T) {
	e := newImagesEnv(t)
	for _, id := range []string{"a", "b"} {
		rel := id + ".mkv"
		if err := os.WriteFile(filepath.Join(e.root, rel), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		e.movie(id, rel)
	}

	err := e.images(filepath.Join(e.root, "no-such-ffmpeg")).Sweep(t.Context())
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("err = %v, want exec.ErrNotFound", err)
	}

	// The first one failed, so the second was not tried, and nothing was saved
	// as a permanent failure: it is tried again once FFmpeg is there.
	entries, err := os.ReadDir(e.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("owners tried = %d, want 1", len(entries))
	}
	for _, o := range entries {
		keys, err := os.ReadDir(filepath.Join(e.dir, o.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if len(keys) != 0 {
			t.Errorf("%s has a saved result: %v", o.Name(), keys)
		}
	}
}

func TestSweepGenerates(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	e := newImagesEnv(t)
	//nolint:gosec // the path comes from LookPath; the output is in a temporary folder
	out, err := exec.CommandContext(t.Context(), ffmpeg, "-hide_banner", "-y", "-f", "lavfi",
		"-i", "testsrc=duration=1:size=320x180:rate=5", "-c:v", "mpeg4", filepath.Join(e.root, "a.mp4")).CombinedOutput()
	if err != nil {
		t.Fatalf("make video: %v: %s", err, out)
	}
	e.movie("a", "a.mp4")
	im := e.images(ffmpeg)

	if err := im.Sweep(t.Context()); err != nil {
		t.Fatal(err)
	}
	if e.colors("a") != 1 {
		t.Error("no dominant color after the sweep")
	}
	src, err := im.sources(t.Context(), "a")
	if err != nil {
		t.Fatal(err)
	}
	card := filepath.Join(e.dir, "a", im.sourceKey(&src), "card-480.webp")
	if !exists(card) {
		t.Fatal("no card after the sweep")
	}

	// What is already there is not generated again.
	if err := os.Remove(card); err != nil {
		t.Fatal(err)
	}
	if err := im.Sweep(t.Context()); err != nil {
		t.Fatal(err)
	}
	if exists(card) {
		t.Error("the second sweep generated the images again")
	}
}
