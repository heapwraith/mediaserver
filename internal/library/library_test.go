package library

import (
	"cmp"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"
	"uuid"

	"mediaserver/internal/db"
	"mediaserver/migrations"
)

// seedMovies creates n movies (every 7th without a year).
func seedMovies(tb testing.TB, n int) Store {
	tb.Helper()
	conn, err := db.Open(tb.Context(), filepath.Join(tb.TempDir(), "t.db"))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = conn.Close() })
	if _, err := db.Migrate(tb.Context(), conn, migrations.FS); err != nil {
		tb.Fatal(err)
	}
	tx, err := conn.BeginTx(tb.Context(), nil)
	if err != nil {
		tb.Fatal(err)
	}
	lib := uuid.NewV7().String()
	exec := func(q string, args ...any) {
		if _, err := tx.ExecContext(tb.Context(), q, args...); err != nil {
			tb.Fatal(err)
		}
	}
	exec(`INSERT INTO libraries (id, name, root_path, created_at) VALUES (?, 'm', '/m', 0)`, lib)
	now := time.Now().UnixMilli()
	for i := range n {
		file, movie := uuid.NewV7().String(), uuid.NewV7().String()
		title := fmt.Sprintf("Movie %05d", (i*7919)%n) // shuffled titles
		var year any = 1950 + i%70
		if i%7 == 0 {
			year = nil
		}
		exec(`INSERT INTO media_files (id, library_id, rel_path, size, mtime_ns, kind, duration_ms, width, height, added_at, updated_at)
		      VALUES (?, ?, ?, 1, 1, 'movie', 5400000, 1920, 1080, ?, ?)`, file, lib, title+".mkv", now+int64(i), now)
		exec(`INSERT INTO movies (id, media_file_id, title, sort_title, norm_title, year, added_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			movie, file, title, title, title, year, now+int64(i))
	}
	if err := tx.Commit(); err != nil {
		tb.Fatal(err)
	}
	return Store{DB: conn}
}

func TestMoviesPaging(t *testing.T) {
	s := seedMovies(t, 50)
	ctx := t.Context()

	var all []Movie
	for off := 0; ; off += 20 {
		page, err := s.Movies(ctx, Page{Sort: "title", Offset: off, Limit: 20})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 50 {
			t.Fatalf("total = %d", page.Total)
		}
		if len(page.Items) == 0 {
			break
		}
		all = append(all, page.Items...)
	}
	if len(all) != 50 || !slices.IsSortedFunc(all, func(a, b Movie) int { return cmp.Compare(a.Title, b.Title) }) {
		t.Fatalf("paged titles not sorted or incomplete: %d", len(all))
	}

	byYear, err := s.Movies(ctx, Page{Sort: "year", Desc: true, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	last := byYear.Items[len(byYear.Items)-1]
	if byYear.Items[0].Year < byYear.Items[1].Year || last.Year != 0 {
		t.Fatalf("year desc: first %d, last %d (unknown years go last)", byYear.Items[0].Year, last.Year)
	}

	added, err := s.Movies(ctx, Page{Sort: "added", Desc: true, Limit: 1})
	if err != nil || added.Items[0].AddedAt < byYear.Items[0].AddedAt {
		t.Fatalf("added desc: %v %+v", err, added.Items)
	}

	for _, p := range []Page{{Sort: "x", Limit: 1}, {Limit: 0}, {Limit: MaxLimit + 1}, {Offset: -1, Limit: 1}} {
		if _, err := s.Movies(ctx, p); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: err = %v", p, err)
		}
	}
	if _, err := s.SeriesByID(ctx, uuid.NewV7().String()); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown series: %v", err)
	}
	if _, err := s.PlayablePath(ctx, uuid.NewV7().String()); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown media: %v", err)
	}
}

// BenchmarkListMovies measures one page of a 10k-movie library (AGENTS.md §8:
// listing < 50 ms p95).
func BenchmarkListMovies(b *testing.B) {
	s := seedMovies(b, 10_000)
	for _, sort := range []string{"title", "added", "year"} {
		b.Run(sort, func(b *testing.B) {
			for i := 0; b.Loop(); i++ {
				p := Page{Sort: sort, Desc: i%2 == 1, Offset: (i * 100) % 9_900, Limit: 100}
				if _, err := s.Movies(b.Context(), p); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
