package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const (
	// TMDBEnabledKey is the app_state key of the TMDB switch (internal/metadata).
	// Off, what was found before stays stored but does not show.
	TMDBEnabledKey = "tmdb_enabled"
	// minInCollection hides franchises the library has one movie of.
	minInCollection = 2
	// Genre rows of the home page.
	maxGenreRows    = 5
	minGenreTitles  = 2
	genreRowMovies  = 10
	genreRowPerKind = 5
)

func (s Store) metadataOn(ctx context.Context) (bool, error) {
	var v string
	err := s.DB.QueryRowContext(ctx, `SELECT value FROM app_state WHERE key = ?`, TMDBEnabledKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read metadata switch: %w", err)
	}
	return v == "1", nil
}

// CollectionRef points to the collection of a movie.
type CollectionRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// genresOf lists the genre names of a movie or series.
func (s Store) genresOf(ctx context.Context, ownerID string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT g.name FROM title_genres tg JOIN genres g ON g.id = tg.genre_id
		WHERE tg.owner_id = ? ORDER BY g.name`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("genres of %s: %w", ownerID, err)
	}
	var out []string
	err = scanAll(rows, func(r *sql.Rows) error {
		var name string
		err := r.Scan(&name)
		out = append(out, name)
		return err
	})
	return out, err
}

// addMovieMeta fills the genres and the collection of a movie, with TMDB on.
func (s Store) addMovieMeta(ctx context.Context, m *Movie) error {
	on, err := s.metadataOn(ctx)
	if err != nil || !on {
		return err
	}
	if m.Genres, err = s.genresOf(ctx, m.ID); err != nil {
		return err
	}
	var c CollectionRef
	err = s.DB.QueryRowContext(ctx, `
		SELECT c.id, c.name
		FROM collection_movies cm JOIN collections c ON c.id = cm.collection_id
		WHERE cm.movie_id = ?
		  AND (SELECT COUNT(*) FROM collection_movies x JOIN movies mv ON mv.id = x.movie_id
		       WHERE x.collection_id = c.id) >= ?`, m.ID, minInCollection).Scan(&c.ID, &c.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("collection of %s: %w", m.ID, err)
	}
	m.Collection = &c
	return nil
}

// GenreCount is a genre with how many titles of a kind have it (a filter button).
type GenreCount struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Genres lists the genres of the movies ("movie") or of the series or animes
// ("series", "anime"), by name. Empty with TMDB off.
func (s Store) Genres(ctx context.Context, kind string) ([]GenreCount, error) {
	out := []GenreCount{}
	on, err := s.metadataOn(ctx)
	if err != nil || !on {
		return out, err
	}
	var rows *sql.Rows
	switch kind {
	case "movie":
		rows, err = s.DB.QueryContext(ctx, `
			SELECT g.id, g.name, COUNT(*)
			FROM title_genres tg JOIN genres g ON g.id = tg.genre_id JOIN movies mv ON mv.id = tg.owner_id
			GROUP BY g.id ORDER BY g.name`)
	case "series", "anime":
		rows, err = s.DB.QueryContext(ctx, `
			SELECT g.id, g.name, COUNT(*)
			FROM title_genres tg JOIN genres g ON g.id = tg.genre_id JOIN series se ON se.id = tg.owner_id
			WHERE se.kind = ?
			GROUP BY g.id ORDER BY g.name`, kind)
	default:
		return out, fmt.Errorf("%w: kind must be movie, series or anime", ErrInvalid)
	}
	if err != nil {
		return out, fmt.Errorf("list genres: %w", err)
	}
	err = scanAll(rows, func(r *sql.Rows) error {
		var g GenreCount
		err := r.Scan(&g.ID, &g.Name, &g.Count)
		out = append(out, g)
		return err
	})
	return out, err
}

// GenreRow is a row of the home page: the latest titles of a genre.
type GenreRow struct {
	ID     int      `json:"id"`
	Name   string   `json:"name"`
	Movies []Movie  `json:"movies"`
	Series []Series `json:"series"`
}

// GenreRows returns the rows of the genres with the most titles. Empty with TMDB off.
func (s Store) GenreRows(ctx context.Context) ([]GenreRow, error) {
	out := []GenreRow{}
	on, err := s.metadataOn(ctx)
	if err != nil || !on {
		return out, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT g.id, g.name
		FROM title_genres tg JOIN genres g ON g.id = tg.genre_id
		WHERE tg.owner_id IN (SELECT id FROM movies UNION SELECT id FROM series)
		GROUP BY g.id HAVING COUNT(*) >= ?
		ORDER BY COUNT(*) DESC, g.name LIMIT ?`, minGenreTitles, maxGenreRows)
	if err != nil {
		return out, fmt.Errorf("genre rows: %w", err)
	}
	if err := scanAll(rows, func(r *sql.Rows) error {
		var g GenreRow
		err := r.Scan(&g.ID, &g.Name)
		out = append(out, g)
		return err
	}); err != nil {
		return out, err
	}
	for i := range out {
		g := &out[i]
		movies, err := s.Movies(ctx, Page{Sort: "added", Desc: true, Limit: genreRowMovies, Genre: g.ID})
		if err != nil {
			return out, err
		}
		g.Movies = movies.Items
		g.Series = []Series{}
		for _, kind := range []string{"series", "anime"} {
			list, err := s.Series(ctx, kind, Page{Sort: "added", Desc: true, Limit: genreRowPerKind, Genre: g.ID})
			if err != nil {
				return out, err
			}
			g.Series = append(g.Series, list.Items...)
		}
	}
	return out, nil
}

// Collection is a franchise card; ImageID is its oldest movie.
type Collection struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Count   int    `json:"count"`
	ImageID string `json:"image_id"`
	Color   string `json:"color,omitzero"`
}

// Collections lists the franchises with two or more movies in the library.
// Empty with TMDB off.
func (s Store) Collections(ctx context.Context) ([]Collection, error) {
	out := []Collection{}
	on, err := s.metadataOn(ctx)
	if err != nil || !on {
		return out, err
	}
	// The MIN makes SQLite take mv.id and ic.color from the oldest movie.
	rows, err := s.DB.QueryContext(ctx, `
		SELECT c.id, c.name, COUNT(*), mv.id, COALESCE(ic.color, ''), MIN(COALESCE(mv.year, 9999))
		FROM collections c
		JOIN collection_movies cm ON cm.collection_id = c.id
		JOIN movies mv ON mv.id = cm.movie_id
		LEFT JOIN image_cache ic ON ic.owner_id = mv.id
		GROUP BY c.id HAVING COUNT(*) >= ?
		ORDER BY c.name`, minInCollection)
	if err != nil {
		return out, fmt.Errorf("list collections: %w", err)
	}
	err = scanAll(rows, func(r *sql.Rows) error {
		var c Collection
		var year int
		err := r.Scan(&c.ID, &c.Name, &c.Count, &c.ImageID, &c.Color, &year)
		out = append(out, c)
		return err
	})
	return out, err
}

// CollectionDetail is a franchise with its movies, oldest first.
type CollectionDetail struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Movies []Movie `json:"movies"`
}

// CollectionByID returns a franchise with two or more movies in the library.
func (s Store) CollectionByID(ctx context.Context, id int) (CollectionDetail, error) {
	d := CollectionDetail{ID: id}
	on, err := s.metadataOn(ctx)
	if err != nil {
		return d, err
	}
	if !on {
		return d, fmt.Errorf("collection %d: %w", id, ErrNotFound)
	}
	err = s.DB.QueryRowContext(ctx, `SELECT name FROM collections WHERE id = ?`, id).Scan(&d.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return d, fmt.Errorf("collection %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return d, fmt.Errorf("collection %d: %w", id, err)
	}
	movies, err := s.Movies(ctx, Page{Sort: "year", Limit: MaxLimit, Collection: id})
	if err != nil {
		return d, err
	}
	if len(movies.Items) < minInCollection {
		return d, fmt.Errorf("collection %d: %w", id, ErrNotFound)
	}
	d.Movies = movies.Items
	return d, nil
}
