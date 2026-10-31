package scanner

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Errors returned by the correction functions.
var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid input")
)

// ReviewItem is a file waiting for a manual decision.
type ReviewItem struct {
	ID      string `json:"id"`
	RelPath string `json:"rel_path"`
	Size    int64  `json:"size"`
	Reason  string `json:"reason"`
	Guess   Guess  `json:"guess"`
}

// ReviewQueue lists ambiguous or unrecognized files, oldest first.
func (s *Scanner) ReviewQueue(ctx context.Context) ([]ReviewItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, rel_path, size, COALESCE(review_reason, ''), COALESCE(guess_json, '{}')
		 FROM media_files WHERE library_id = ? AND status = 'review' ORDER BY added_at, rel_path`, s.libraryID)
	if err != nil {
		return nil, fmt.Errorf("review queue: %w", err)
	}
	defer rows.Close()
	out := []ReviewItem{}
	for rows.Next() {
		var it ReviewItem
		var guess string
		if err := rows.Scan(&it.ID, &it.RelPath, &it.Size, &it.Reason, &guess); err != nil {
			return nil, fmt.Errorf("review queue row: %w", err)
		}
		if err := json.Unmarshal([]byte(guess), &it.Guess); err != nil {
			return nil, fmt.Errorf("review guess %s: %w", it.ID, err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("review queue: %w", err)
	}
	return out, nil
}

// Correction is a manual fix for one file (PUT /api/review/{fileId}).
type Correction struct {
	Kind    string `json:"kind"` // movie | series | anime | ignored
	Title   string `json:"title,omitempty"`
	Year    int    `json:"year,omitempty"`
	Season  *int   `json:"season,omitempty"`
	Episode *int   `json:"episode,omitempty"`
}

// Validate checks the correction and trims the title.
func (c *Correction) Validate() error {
	c.Title = strings.TrimSpace(c.Title)
	switch c.Kind {
	case "ignored":
		return nil
	case "movie", "series", "anime":
	default:
		return fmt.Errorf("%w: kind must be movie, series, anime or ignored", ErrInvalid)
	}
	if c.Title == "" || len(c.Title) > 300 {
		return fmt.Errorf("%w: title is required (max 300 characters)", ErrInvalid)
	}
	if c.Year != 0 && (c.Year < 1870 || c.Year > time.Now().Year()+5) {
		return fmt.Errorf("%w: year out of range", ErrInvalid)
	}
	if c.Kind == "movie" {
		if c.Season != nil || c.Episode != nil {
			return fmt.Errorf("%w: a movie has no season or episode", ErrInvalid)
		}
		return nil
	}
	if c.Season == nil || *c.Season < 0 || *c.Season > 500 {
		return fmt.Errorf("%w: season is required (0-500, 0 = specials)", ErrInvalid)
	}
	if c.Episode == nil || *c.Episode < 1 || *c.Episode > 100000 {
		return fmt.Errorf("%w: episode is required (1-100000)", ErrInvalid)
	}
	return nil
}

// FileState is where a file ended up in the library.
type FileState struct {
	ID        string `json:"id"`
	RelPath   string `json:"rel_path"`
	Status    string `json:"status"` // ok | review | ignored
	Kind      string `json:"kind"`   // movie | episode | trailer | extra | unknown
	Reason    string `json:"reason,omitempty"`
	MovieID   string `json:"movie_id,omitempty"`
	SeriesID  string `json:"series_id,omitempty"`
	EpisodeID string `json:"episode_id,omitempty"`
}

// FileState reads the library position of a file.
func (s *Scanner) FileState(ctx context.Context, id string) (FileState, error) {
	var st FileState
	var movieID, seriesID, episodeID sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT f.id, f.rel_path, f.status, f.kind, COALESCE(f.review_reason, ''), m.id, e.series_id, e.id
		 FROM media_files f
		 LEFT JOIN movies m ON m.media_file_id = f.id
		 LEFT JOIN episodes e ON e.media_file_id = f.id
		 WHERE f.id = ?`, id).Scan(&st.ID, &st.RelPath, &st.Status, &st.Kind, &st.Reason, &movieID, &seriesID, &episodeID)
	if errors.Is(err, sql.ErrNoRows) {
		return st, fmt.Errorf("file %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return st, fmt.Errorf("file state %s: %w", id, err)
	}
	st.MovieID, st.SeriesID, st.EpisodeID = movieID.String, seriesID.String, episodeID.String
	return st, nil
}

type fileKey struct {
	rel  string
	size int64
	hash string
}

func (s *Scanner) fileKey(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string,
) (fileKey, error) {
	var k fileKey
	err := q.QueryRowContext(ctx, `SELECT rel_path, size, COALESCE(partial_hash, '') FROM media_files WHERE id = ?`, id).
		Scan(&k.rel, &k.size, &k.hash)
	if errors.Is(err, sql.ErrNoRows) {
		return k, fmt.Errorf("file %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return k, fmt.Errorf("load file %s: %w", id, err)
	}
	return k, nil
}

// Correct saves a manual correction as an override (it wins over the parser
// on every future scan, and follows the file if it is renamed) and rebuilds
// the library.
func (s *Scanner) Correct(ctx context.Context, fileID string, c Correction) (FileState, error) {
	if err := c.Validate(); err != nil {
		return FileState{}, err
	}
	k, err := s.fileKey(ctx, s.db, fileID)
	if err != nil {
		return FileState{}, err
	}
	var year any
	if c.Year != 0 {
		year = c.Year
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO overrides (rel_path, size, partial_hash, kind, title, year, season, episode, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (rel_path, size) DO UPDATE SET partial_hash = excluded.partial_hash, kind = excluded.kind,
		   title = excluded.title, year = excluded.year, season = excluded.season, episode = excluded.episode,
		   updated_at = excluded.updated_at`,
		k.rel, k.size, nullStr(k.hash), c.Kind, nullStr(c.Title), year, intPtr(c.Season), intPtr(c.Episode),
		time.Now().UnixMilli())
	if err != nil {
		return FileState{}, fmt.Errorf("save override: %w", err)
	}
	if err := s.Rematerialize(ctx); err != nil {
		return FileState{}, err
	}
	return s.FileState(ctx, fileID)
}

// setSeriesTitle writes a kind+title override for files, keeping any
// season/episode correction they already have.
func (s *Scanner) setSeriesTitle(ctx context.Context, fileIDs []string, kind, title string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UnixMilli()
	for _, id := range fileIDs {
		k, err := s.fileKey(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO overrides (rel_path, size, partial_hash, kind, title, updated_at) VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT (rel_path, size) DO UPDATE SET kind = excluded.kind, title = excluded.title,
			   partial_hash = excluded.partial_hash, updated_at = excluded.updated_at`,
			k.rel, k.size, nullStr(k.hash), kind, title, now); err != nil {
			return fmt.Errorf("save override: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit overrides: %w", err)
	}
	return nil
}

type seriesInfo struct {
	id, kind, title, norm string
}

func (s *Scanner) series(ctx context.Context, id string) (seriesInfo, error) {
	si := seriesInfo{id: id}
	err := s.db.QueryRowContext(ctx, `SELECT kind, title, norm_title FROM series WHERE id = ?`, id).
		Scan(&si.kind, &si.title, &si.norm)
	if errors.Is(err, sql.ErrNoRows) {
		return si, fmt.Errorf("series %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return si, fmt.Errorf("load series %s: %w", id, err)
	}
	return si, nil
}

func (s *Scanner) seriesFiles(ctx context.Context, seriesID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT media_file_id FROM episodes WHERE series_id = ?`, seriesID)
	if err != nil {
		return nil, fmt.Errorf("series files: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("series files: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("series files: %w", err)
	}
	return out, nil
}

// MergeSeries moves every episode of sourceIDs into targetID. It returns the
// target series ID.
func (s *Scanner) MergeSeries(ctx context.Context, sourceIDs []string, targetID string) (string, error) {
	if len(sourceIDs) == 0 || slices.Contains(sourceIDs, targetID) {
		return "", fmt.Errorf("%w: source_ids must be non-empty and must not contain target_id", ErrInvalid)
	}
	target, err := s.series(ctx, targetID)
	if err != nil {
		return "", err
	}
	var files []string
	for _, src := range sourceIDs {
		if _, err := s.series(ctx, src); err != nil {
			return "", err
		}
		f, err := s.seriesFiles(ctx, src)
		if err != nil {
			return "", err
		}
		files = append(files, f...)
	}
	// The target keeps its name too, even if a later rescan finds a
	// "cleaner" title among the merged files.
	own, err := s.seriesFiles(ctx, targetID)
	if err != nil {
		return "", err
	}
	if err := s.setSeriesTitle(ctx, append(files, own...), target.kind, target.title); err != nil {
		return "", err
	}
	if err := s.Rematerialize(ctx); err != nil {
		return "", err
	}
	return s.seriesIDByNorm(ctx, target.norm)
}

// SplitSeries moves fileIDs (episodes of seriesID) into a new series called
// title. It returns the new series ID.
func (s *Scanner) SplitSeries(ctx context.Context, seriesID string, fileIDs []string, title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 300 || len(fileIDs) == 0 {
		return "", fmt.Errorf("%w: title and file_ids are required", ErrInvalid)
	}
	src, err := s.series(ctx, seriesID)
	if err != nil {
		return "", err
	}
	if NormTitle(title) == src.norm {
		return "", fmt.Errorf("%w: the new title must differ from the current one", ErrInvalid)
	}
	own, err := s.seriesFiles(ctx, seriesID)
	if err != nil {
		return "", err
	}
	for _, id := range fileIDs {
		if !slices.Contains(own, id) {
			return "", fmt.Errorf("%w: file %s is not an episode of series %s", ErrInvalid, id, seriesID)
		}
	}
	if len(fileIDs) == len(own) {
		return "", fmt.Errorf("%w: cannot split every episode; rename the series instead", ErrInvalid)
	}
	if err := s.setSeriesTitle(ctx, fileIDs, src.kind, title); err != nil {
		return "", err
	}
	if err := s.Rematerialize(ctx); err != nil {
		return "", err
	}
	return s.seriesIDByNorm(ctx, NormTitle(title))
}

func (s *Scanner) seriesIDByNorm(ctx context.Context, norm string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM series WHERE norm_title = ?`, norm).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("find series %q: %w", norm, err)
	}
	return id, nil
}

func intPtr(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
