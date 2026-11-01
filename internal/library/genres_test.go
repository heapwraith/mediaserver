package library

import (
	"fmt"
	"testing"
)

func TestGenresAndCollections(t *testing.T) {
	s := seedMovies(t, 4)
	ctx := t.Context()
	series := seedSeries(t, s, 1, 1)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.DB.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	movies, err := s.Movies(ctx, Page{Sort: "added", Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	id := func(i int) string { return movies.Items[i].ID }
	seriesID := mustSeriesOf(t, s, series[0][0])

	exec(`INSERT INTO genres (id, name) VALUES (1, 'Ação'), (2, 'Drama'), (3, 'Só um')`)
	for _, g := range [][2]any{{id(0), 1}, {id(1), 1}, {seriesID, 1}, {id(2), 2}, {seriesID, 2}, {id(3), 3}} {
		exec(`INSERT INTO title_genres (owner_id, genre_id) VALUES (?, ?)`, g[0], g[1])
	}
	exec(`INSERT INTO collections (id, name) VALUES (10, 'Franquia'), (20, 'Sozinho')`)
	exec(`INSERT INTO collection_movies (collection_id, movie_id) VALUES (10, ?), (10, ?), (20, ?)`, id(0), id(1), id(2))

	// Off (the default): everything found before stays hidden.
	check := func(when string, wantRows, wantCollections, wantGenres int) {
		t.Helper()
		rows, err := s.GenreRows(ctx)
		if err != nil || len(rows) != wantRows {
			t.Errorf("%s: rows = %+v %v, want %d", when, rows, err, wantRows)
		}
		cols, err := s.Collections(ctx)
		if err != nil || len(cols) != wantCollections {
			t.Errorf("%s: collections = %+v %v, want %d", when, cols, err, wantCollections)
		}
		gs, err := s.Genres(ctx, "movie")
		if err != nil || len(gs) != wantGenres {
			t.Errorf("%s: genres = %+v %v, want %d", when, gs, err, wantGenres)
		}
	}
	check("off", 0, 0, 0)
	if m, err := s.MovieByID(ctx, id(0)); err != nil || m.Genres != nil || m.Collection != nil {
		t.Errorf("movie while off = %+v %v", m, err)
	}
	if _, err := s.CollectionByID(ctx, 10); err == nil {
		t.Error("collection shown while off")
	}

	exec(`INSERT INTO app_state (key, value) VALUES (?, '1')`, TMDBEnabledKey)
	check("on", 2, 1, 3) // "Só um" has one title: a button, not a row

	rows, err := s.GenreRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Name != "Ação" || len(rows[0].Movies) != 2 || len(rows[0].Series) != 1 || rows[0].Series[0].ID != seriesID {
		t.Errorf("first row = %+v", rows[0])
	}
	if rows[1].Name != "Drama" || len(rows[1].Movies) != 1 || len(rows[1].Series) != 1 {
		t.Errorf("second row = %+v", rows[1])
	}

	// The filter of the library pages.
	list, err := s.Movies(ctx, Page{Sort: "title", Limit: 10, Genre: 1})
	if err != nil || list.Total != 2 || len(list.Items) != 2 {
		t.Errorf("movies of a genre = %+v %v", list, err)
	}
	sl, err := s.Series(ctx, "series", Page{Sort: "title", Limit: 10, Genre: 2})
	if err != nil || sl.Total != 1 || sl.Items[0].ID != seriesID {
		t.Errorf("series of a genre = %+v %v", sl, err)
	}
	if gs, err := s.Genres(ctx, "series"); err != nil || len(gs) != 2 || gs[0].Count != 1 {
		t.Errorf("series genres = %+v %v", gs, err)
	}
	if _, err := s.Genres(ctx, "x"); err == nil {
		t.Error("unknown kind accepted")
	}

	// Details.
	m, err := s.MovieByID(ctx, id(0))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(m.Genres) != "[Ação]" || m.Collection == nil || m.Collection.ID != 10 {
		t.Errorf("movie = %+v", m)
	}
	// A franchise with one movie here does not count.
	if m, _ := s.MovieByID(ctx, id(2)); m.Collection != nil {
		t.Errorf("collection of one movie = %+v", m.Collection)
	}
	if _, err := s.CollectionByID(ctx, 20); err == nil {
		t.Error("collection of one movie shown")
	}
	d, err := s.SeriesByID(ctx, seriesID)
	if err != nil || fmt.Sprint(d.Genres) != "[Ação Drama]" {
		t.Errorf("series = %v %v", d.Genres, err)
	}

	// Collections.
	cols, err := s.Collections(ctx)
	if err != nil || cols[0].Name != "Franquia" || cols[0].Count != 2 || cols[0].ImageID == "" {
		t.Fatalf("collections = %+v %v", cols, err)
	}
	c, err := s.CollectionByID(ctx, 10)
	if err != nil || c.Name != "Franquia" || len(c.Movies) != 2 {
		t.Fatalf("collection = %+v %v", c, err)
	}

	// A movie out of the library leaves its collection without counting.
	exec(`DELETE FROM movies WHERE id = ?`, id(1))
	if cols, _ := s.Collections(ctx); len(cols) != 0 {
		t.Errorf("collection left with one movie = %+v", cols)
	}
}
