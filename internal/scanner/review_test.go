package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCorrectionSurvivesRescanAndRename(t *testing.T) {
	e := newEnv(t)
	for _, rel := range sampleLibrary {
		e.write(t, rel)
	}
	e.scan(t)
	ctx := context.Background()

	queue, err := e.sc.ReviewQueue(ctx)
	if err != nil || len(queue) != 2 {
		t.Fatalf("review queue = %+v, %v", queue, err)
	}
	id := e.fileID(t, "Random Video.mkv")

	st, err := e.sc.Correct(ctx, id, Correction{Kind: "movie", Title: "Férias na Praia", Year: 2019})
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "ok" || st.MovieID == "" {
		t.Fatalf("after correction: %+v", st)
	}
	movieTitle := func() string {
		return e.str(t, `SELECT m.title || ' ' || m.year FROM movies m WHERE m.media_file_id = ?`, id)
	}
	if got := movieTitle(); got != "Férias na Praia 2019" {
		t.Fatalf("movie = %q", got)
	}
	if q, _ := e.sc.ReviewQueue(ctx); len(q) != 1 {
		t.Fatalf("review queue after correction has %d items, want 1", len(q))
	}

	// Rescan: the parser would say "review", the override wins.
	e.scan(t)
	if got := movieTitle(); got != "Férias na Praia 2019" {
		t.Fatalf("after rescan movie = %q", got)
	}

	// Rename + move: same file ID, correction still applied.
	if err := os.MkdirAll(filepath.Join(e.root, "Videos"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(e.root, "Random Video.mkv"), filepath.Join(e.root, "Videos", "praia.mkv")); err != nil {
		t.Fatal(err)
	}
	st2 := e.scan(t)
	if st2.Moved != 1 {
		t.Fatalf("rename not detected as move: %+v", st2)
	}
	if got := e.fileID(t, "Videos/praia.mkv"); got != id {
		t.Fatalf("renamed file id %s, want %s", got, id)
	}
	if got := movieTitle(); got != "Férias na Praia 2019" {
		t.Fatalf("after rename movie = %q", got)
	}

	// Episode correction.
	showID := e.fileID(t, "Show - 05.mkv")
	st, err = e.sc.Correct(ctx, showID, Correction{Kind: "series", Title: "The Show", Season: new(2), Episode: new(5)})
	if err != nil {
		t.Fatal(err)
	}
	if st.EpisodeID == "" {
		t.Fatalf("episode correction: %+v", st)
	}
	if got := e.str(t, `SELECT s.number || 'x' || e.number FROM episodes e JOIN seasons s ON s.id = e.season_id WHERE e.media_file_id = ?`, showID); got != "2x5" {
		t.Fatalf("episode = %s, want 2x5", got)
	}

	// Ignore.
	if st, err = e.sc.Correct(ctx, showID, Correction{Kind: "ignored"}); err != nil || st.Status != "ignored" {
		t.Fatalf("ignore: %+v %v", st, err)
	}
}

func TestCorrectionValidation(t *testing.T) {
	e := newEnv(t)
	e.write(t, "Random Video.mkv")
	e.scan(t)
	id := e.fileID(t, "Random Video.mkv")
	tests := []struct {
		name string
		c    Correction
	}{
		{"bad kind", Correction{Kind: "documentary", Title: "X"}},
		{"no title", Correction{Kind: "movie"}},
		{"year out of range", Correction{Kind: "movie", Title: "X", Year: 1200}},
		{"movie with episode", Correction{Kind: "movie", Title: "X", Episode: new(1)}},
		{"series without episode", Correction{Kind: "series", Title: "X", Season: new(1)}},
		{"series without season", Correction{Kind: "series", Title: "X", Episode: new(1)}},
		{"negative season", Correction{Kind: "anime", Title: "X", Season: new(-1), Episode: new(1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := e.sc.Correct(context.Background(), id, tt.c); !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
	if _, err := e.sc.Correct(context.Background(), "0190f3c0-0000-7000-8000-000000000000",
		Correction{Kind: "ignored"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown file: err = %v, want ErrNotFound", err)
	}
}

func TestMergeAndSplit(t *testing.T) {
	e := newEnv(t)
	for _, rel := range []string{
		"Shingeki no Kyojin/[Group] Shingeki no Kyojin - 01.mkv",
		"Shingeki no Kyojin/[Group] Shingeki no Kyojin - 02.mkv",
		"[Other] Attack on Titan - 03.mkv",
		"[Other] Attack on Titan - 04.mkv",
	} {
		e.write(t, rel)
	}
	e.scan(t)
	ctx := context.Background()
	if n := e.count(t, `SELECT count(*) FROM series`); n != 2 {
		t.Fatalf("series = %d, want 2", n)
	}
	snk := e.str(t, `SELECT id FROM series WHERE norm_title = 'shingeki no kyojin'`)
	aot := e.str(t, `SELECT id FROM series WHERE norm_title = 'attack on titan'`)

	got, err := e.sc.MergeSeries(ctx, []string{aot}, snk)
	if err != nil {
		t.Fatal(err)
	}
	if got != snk {
		t.Fatalf("merge returned %s, want target %s", got, snk)
	}
	if n := e.count(t, `SELECT count(*) FROM series`); n != 1 {
		t.Fatalf("after merge series = %d, want 1", n)
	}
	if n := e.count(t, `SELECT count(*) FROM episodes WHERE series_id = ?`, snk); n != 4 {
		t.Fatalf("after merge episodes = %d, want 4", n)
	}
	e.scan(t) // survives a rescan
	if n := e.count(t, `SELECT count(*) FROM series`); n != 1 {
		t.Fatalf("merge lost after rescan")
	}

	// Split episodes 3 and 4 back out under a new name.
	ids := []string{e.fileID(t, "[Other] Attack on Titan - 03.mkv"), e.fileID(t, "[Other] Attack on Titan - 04.mkv")}
	newID, err := e.sc.SplitSeries(ctx, snk, ids, "Attack on Titan (dublado)")
	if err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT count(*) FROM episodes WHERE series_id = ?`, newID); n != 2 {
		t.Fatalf("split series has %d episodes, want 2", n)
	}
	if got := e.str(t, `SELECT kind FROM series WHERE id = ?`, newID); got != "anime" {
		t.Fatalf("split series kind = %s, want anime", got)
	}
	e.scan(t)
	if n := e.count(t, `SELECT count(*) FROM series`); n != 2 {
		t.Fatalf("split lost after rescan")
	}

	// Invalid requests.
	if _, err := e.sc.MergeSeries(ctx, []string{snk}, snk); !errors.Is(err, ErrInvalid) {
		t.Errorf("merge into itself: %v", err)
	}
	if _, err := e.sc.SplitSeries(ctx, snk, []string{ids[0]}, "Other"); !errors.Is(err, ErrInvalid) {
		t.Errorf("split a file of another series: %v", err)
	}
	if _, err := e.sc.MergeSeries(ctx, []string{"0190f3c0-0000-7000-8000-000000000000"}, snk); !errors.Is(err, ErrNotFound) {
		t.Errorf("merge unknown series: %v", err)
	}
}
