package metadata

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"mediaserver/internal/library"
	"mediaserver/internal/probe"
	"mediaserver/internal/scanner"
)

const (
	keyEnabled = library.TMDBEnabledKey
	keyToken   = "tmdb_key"
	// pace is the wait between titles: a library is looked up once, slowly.
	pace = 250 * time.Millisecond
	// maxCandidates are the choices kept for Revisar.
	maxCandidates = 5
	maxKeyLen     = 1024
	// yearSlack: a release year often differs by one between countries.
	yearSlack = 1
)

// Codes of the last run, for the settings page.
const (
	CodeInvalidKey  = "invalid_key"
	CodeUnavailable = "unavailable"
)

var (
	reTagTMDB = regexp.MustCompile(`^(?:(movie|tv)/)?(\d+)$`)
	reTagIMDB = regexp.MustCompile(`^tt\d+$`)
)

// Syncer finds, in the background, the genres and collections of the library.
// With TMDB off (the default) it makes no network call at all.
type Syncer struct {
	db   *sql.DB
	log  *slog.Logger
	wake chan struct{}
	base string
	http *http.Client
	pace time.Duration

	mu      sync.Mutex
	running bool
	lastErr string
}

// New prepares the syncer; Run must run for its lifetime.
func New(db *sql.DB, logger *slog.Logger) *Syncer {
	return &Syncer{
		db: db, log: logger, wake: make(chan struct{}, 1), base: defaultBase, pace: pace,
		http: &http.Client{Timeout: timeout},
	}
}

// Trigger asks for a sync (after a scan or a settings change); it never blocks.
func (s *Syncer) Trigger() {
	select {
	case s.wake <- struct{}{}:
	default: // one is already queued
	}
}

// Run syncs on every Trigger until ctx ends.
func (s *Syncer) Run(ctx context.Context) {
	s.Trigger() // titles added while the server was off
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
			if err := s.Sync(ctx); err != nil && ctx.Err() == nil {
				s.log.Warn("metadata sync failed", "err", err)
			}
		}
	}
}

// Status is what the settings page shows. It never has the key.
type Status struct {
	Enabled   bool   `json:"enabled"`
	HasKey    bool   `json:"has_key"`
	Running   bool   `json:"running"`
	Matched   int    `json:"matched"`
	Review    int    `json:"review"`
	LastError string `json:"last_error,omitzero"`
}

// Status reads the switch, the counts and the result of the last run.
func (s *Syncer) Status(ctx context.Context) (Status, error) {
	var st Status
	enabled, key, err := s.settings(ctx)
	if err != nil {
		return st, err
	}
	st.Enabled, st.HasKey = enabled, key != ""
	err = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(status = 'matched'), 0), COALESCE(SUM(status = 'review'), 0)
		FROM title_meta`).Scan(&st.Matched, &st.Review)
	if err != nil {
		return st, fmt.Errorf("metadata status: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st.Running, st.LastError = s.running, s.lastErr
	return st, nil
}

// settings reads the switch and the key.
func (s *Syncer) settings(ctx context.Context) (enabled bool, key string, err error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value FROM app_state WHERE key IN (?, ?)`, keyEnabled, keyToken)
	if err != nil {
		return false, "", fmt.Errorf("read metadata settings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return false, "", fmt.Errorf("read metadata settings: %w", err)
		}
		if k == keyEnabled {
			enabled = v == "1"
		} else {
			key = v
		}
	}
	if err := rows.Err(); err != nil {
		return false, "", fmt.Errorf("read metadata settings: %w", err)
	}
	return enabled, key, nil
}

// SetEnabled turns the lookups on or off.
func (s *Syncer) SetEnabled(ctx context.Context, on bool) error {
	v := "0"
	if on {
		v = "1"
	}
	return s.setState(ctx, keyEnabled, v)
}

// SetKey saves the key; an empty one removes it. A new key clears the last error.
func (s *Syncer) SetKey(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if len(key) > maxKeyLen || strings.ContainsFunc(key, func(r rune) bool { return r <= ' ' || r > '~' }) {
		return fmt.Errorf("%w: that does not look like a TMDB key", ErrInvalid)
	}
	if err := s.setState(ctx, keyToken, key); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = ""
	return nil
}

func (s *Syncer) setState(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_state (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("save %s: %w", key, err)
	}
	return nil
}

// title is a movie or series to look up.
type title struct {
	ownerType string // movie | series
	id        string
	name      string
	year      int
	lookupKey string
}

func (t title) kind() string {
	if t.ownerType == "movie" {
		return kindMovie
	}
	return kindTV
}

// Sync looks up every title without an answer yet.
func (s *Syncer) Sync(ctx context.Context) error {
	if enabled, key, err := s.settings(ctx); err != nil || !enabled || key == "" {
		return err
	}
	s.setRun(true, "")
	err := s.sync(ctx)
	code := ""
	switch {
	case errors.Is(err, ErrUnauthorized):
		code = CodeInvalidKey
	case err != nil && ctx.Err() == nil:
		code = CodeUnavailable
	}
	s.setRun(false, code)
	return err
}

func (s *Syncer) setRun(running bool, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running, s.lastErr = running, lastErr
}

func (s *Syncer) client(key string) client {
	return client{base: s.base, key: key, http: s.http}
}

func (s *Syncer) sync(ctx context.Context) error {
	todo, err := s.pending(ctx)
	if err != nil {
		return err
	}
	matched, review := 0, 0
	for i, t := range todo {
		// Read for each title: turning it off stops the calls at once.
		enabled, key, err := s.settings(ctx)
		if err != nil {
			return err
		}
		if !enabled || key == "" {
			return nil
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.pace):
			}
		}
		ok, err := s.process(ctx, s.client(key), t)
		if err != nil {
			// The key is wrong, or TMDB or the network are out: nothing was
			// written, so the next scan tries the same titles again.
			return err
		}
		if ok {
			matched++
		} else {
			review++
		}
	}
	if err := s.cleanup(ctx); err != nil {
		return err
	}
	if len(todo) > 0 {
		s.log.Info("metadata synced", "matched", matched, "review", review)
	}
	return nil
}

// pending lists the movies and series without an answer for their current title.
func (s *Syncer) pending(ctx context.Context) ([]title, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT 'movie', mv.id, mv.title, COALESCE(mv.year, 0), mv.norm_title || '|' || COALESCE(mv.year, 0)
		FROM movies mv LEFT JOIN title_meta tm ON tm.owner_id = mv.id
		WHERE tm.owner_id IS NULL OR tm.lookup_key <> mv.norm_title || '|' || COALESCE(mv.year, 0)
		UNION ALL
		SELECT 'series', se.id, se.title, COALESCE(se.year, 0), se.norm_title || '|' || COALESCE(se.year, 0)
		FROM series se LEFT JOIN title_meta tm ON tm.owner_id = se.id
		WHERE tm.owner_id IS NULL OR tm.lookup_key <> se.norm_title || '|' || COALESCE(se.year, 0)`)
	if err != nil {
		return nil, fmt.Errorf("list titles: %w", err)
	}
	defer rows.Close()
	var out []title
	for rows.Next() {
		var t title
		if err := rows.Scan(&t.ownerType, &t.id, &t.name, &t.year, &t.lookupKey); err != nil {
			return nil, fmt.Errorf("list titles: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list titles: %w", err)
	}
	return out, nil
}

// process identifies a title and saves the answer; false: it went to Revisar.
func (s *Syncer) process(ctx context.Context, c client, t title) (bool, error) {
	id, choices, err := s.identify(ctx, c, t)
	if err != nil {
		return false, err
	}
	if id > 0 {
		d, err := c.details(ctx, t.kind(), id)
		if err == nil {
			return true, s.saveMatch(ctx, t, id, d)
		}
		if !errors.Is(err, ErrNotFound) {
			return false, err
		}
		// A tag with a code TMDB does not know: the user decides.
	}
	return false, s.saveReview(ctx, t, choices)
}

// identify finds the TMDB id: the tag in the file, the IMDb tag, or a search
// that has only one possible answer. Otherwise it returns the choices.
func (s *Syncer) identify(ctx context.Context, c client, t title) (int, []Candidate, error) {
	tmdb, imdb, err := s.tags(ctx, t)
	if err != nil {
		return 0, nil, err
	}
	if id := tagID(tmdb, t.kind()); id > 0 {
		return id, nil, nil
	}
	if reTagIMDB.MatchString(imdb) {
		found, err := c.find(ctx, imdb, t.kind())
		if err != nil && !errors.Is(err, ErrNotFound) {
			return 0, nil, err
		}
		if len(found) == 1 {
			return found[0].ID, nil, nil
		}
	}
	found, err := c.search(ctx, t.kind(), t.name)
	if err != nil {
		return 0, nil, err
	}
	if id := sureMatch(found, t.name, t.year); id > 0 {
		return id, nil, nil
	}
	return 0, found[:min(len(found), maxCandidates)], nil
}

// tagID reads a TMDB tag ("tv/136315", "movie/1", "136315"); 0 when it is of
// another kind of title or is not a code.
func tagID(tag, kind string) int {
	m := reTagTMDB.FindStringSubmatch(strings.TrimSpace(tag))
	if m == nil || (m[1] != "" && m[1] != kind) {
		return 0
	}
	id, err := strconv.Atoi(m[2])
	if err != nil {
		return 0
	}
	return id
}

// sureMatch is the id of the only result with this very title and a compatible
// year; 0 when there is none or more than one. A wrong genre is worse than none.
func sureMatch(found []Candidate, name string, year int) int {
	norm := scanner.NormTitle(name)
	id := 0
	for _, c := range found {
		if scanner.NormTitle(c.Title) != norm && scanner.NormTitle(c.Original) != norm {
			continue
		}
		if year != 0 && (c.Year < year-yearSlack || c.Year > year+yearSlack) {
			continue
		}
		if id != 0 {
			return 0
		}
		id = c.ID
	}
	return id
}

// tags reads the TMDB and IMDb tags release groups leave in the files of a
// title (from the ffprobe answer the scanner kept: nothing is read from disk).
func (s *Syncer) tags(ctx context.Context, t title) (tmdb, imdb string, err error) {
	query := `SELECT mf.probe_json FROM movies mv JOIN media_files mf ON mf.id = mv.media_file_id
		WHERE mv.id = ? AND mf.probe_json IS NOT NULL`
	if t.ownerType == "series" {
		query = `SELECT mf.probe_json FROM episodes e JOIN media_files mf ON mf.id = e.media_file_id
			WHERE e.series_id = ? AND mf.probe_json IS NOT NULL`
	}
	rows, err := s.db.QueryContext(ctx, query, t.id)
	if err != nil {
		return "", "", fmt.Errorf("tags of %s: %w", t.id, err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return "", "", fmt.Errorf("tags of %s: %w", t.id, err)
		}
		res, err := probe.Parse([]byte(raw), "")
		if err != nil {
			continue
		}
		if tmdb == "" {
			tmdb = res.Tags["tmdb"]
		}
		if imdb == "" {
			imdb = res.Tags["imdb"]
		}
		if tmdb != "" {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return "", "", fmt.Errorf("tags of %s: %w", t.id, err)
	}
	return tmdb, imdb, nil
}

// saveMatch stores the genres and the collection of a title, replacing what it had.
func (s *Syncer) saveMatch(ctx context.Context, t title, id int, d details) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save %s: %w", t.id, err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertMeta(ctx, tx, t, "matched", id, nil); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM title_genres WHERE owner_id = ?`, t.id); err != nil {
		return fmt.Errorf("save %s: %w", t.id, err)
	}
	for _, g := range d.Genres {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO genres (id, name) VALUES (?, ?)
			ON CONFLICT (id) DO UPDATE SET name = excluded.name`, g.ID, g.Name); err != nil {
			return fmt.Errorf("save genre %d: %w", g.ID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO title_genres (owner_id, genre_id) VALUES (?, ?)`, t.id, g.ID); err != nil {
			return fmt.Errorf("save %s: %w", t.id, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM collection_movies WHERE movie_id = ?`, t.id); err != nil {
		return fmt.Errorf("save %s: %w", t.id, err)
	}
	if c := d.Collection; c != nil && t.ownerType == "movie" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO collections (id, name) VALUES (?, ?)
			ON CONFLICT (id) DO UPDATE SET name = excluded.name`, c.ID, c.Name); err != nil {
			return fmt.Errorf("save collection %d: %w", c.ID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO collection_movies (collection_id, movie_id) VALUES (?, ?)`, c.ID, t.id); err != nil {
			return fmt.Errorf("save %s: %w", t.id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save %s: %w", t.id, err)
	}
	return nil
}

// saveReview sends a title to Revisar, without genres.
func (s *Syncer) saveReview(ctx context.Context, t title, choices []Candidate) error {
	if choices == nil {
		choices = []Candidate{}
	}
	data, err := json.Marshal(choices)
	if err != nil {
		return fmt.Errorf("encode candidates: %w", err)
	}
	return s.replaceAsUnmatched(ctx, t, "review", data)
}

// replaceAsUnmatched stores a status without genres or collection.
func (s *Syncer) replaceAsUnmatched(ctx context.Context, t title, status string, choices []byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save %s: %w", t.id, err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertMeta(ctx, tx, t, status, 0, choices); err != nil {
		return err
	}
	for _, q := range []string{
		`DELETE FROM title_genres WHERE owner_id = ?`,
		`DELETE FROM collection_movies WHERE movie_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, t.id); err != nil {
			return fmt.Errorf("save %s: %w", t.id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save %s: %w", t.id, err)
	}
	return nil
}

func upsertMeta(ctx context.Context, tx *sql.Tx, t title, status string, id int, choices []byte) error {
	var tmdb, cands any
	if id > 0 {
		tmdb = id
	}
	if choices != nil {
		cands = string(choices)
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO title_meta (owner_id, owner_type, status, tmdb_id, lookup_key, candidates, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (owner_id) DO UPDATE SET status = excluded.status, tmdb_id = excluded.tmdb_id,
			lookup_key = excluded.lookup_key, candidates = excluded.candidates, updated_at = excluded.updated_at`,
		t.id, t.ownerType, status, tmdb, t.lookupKey, cands, time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("save %s: %w", t.id, err)
	}
	return nil
}

// cleanup removes what belongs to titles that left the library.
func (s *Syncer) cleanup(ctx context.Context) error {
	const owners = `(SELECT id FROM movies UNION SELECT id FROM series)`
	for _, q := range []string{
		`DELETE FROM title_meta WHERE owner_id NOT IN ` + owners,
		`DELETE FROM title_genres WHERE owner_id NOT IN ` + owners,
		`DELETE FROM collection_movies WHERE movie_id NOT IN (SELECT id FROM movies)`,
		`DELETE FROM collections WHERE id NOT IN (SELECT collection_id FROM collection_movies)`,
		`DELETE FROM genres WHERE id NOT IN (SELECT genre_id FROM title_genres)`,
	} {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("clean metadata: %w", err)
		}
	}
	return nil
}

// ReviewItem is a title the user has to identify in Revisar.
type ReviewItem struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"` // movie | series
	Title      string      `json:"title"`
	Year       int         `json:"year,omitzero"`
	Candidates []Candidate `json:"candidates"`
}

// Review lists the titles waiting for the user.
func (s *Syncer) Review(ctx context.Context) ([]ReviewItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT tm.owner_id, tm.owner_type, COALESCE(mv.title, se.title), COALESCE(mv.year, se.year, 0),
		       COALESCE(tm.candidates, '[]')
		FROM title_meta tm
		LEFT JOIN movies mv ON mv.id = tm.owner_id
		LEFT JOIN series se ON se.id = tm.owner_id
		WHERE tm.status = 'review' AND COALESCE(mv.id, se.id) IS NOT NULL
		ORDER BY 3`)
	if err != nil {
		return nil, fmt.Errorf("metadata review: %w", err)
	}
	defer rows.Close()
	out := []ReviewItem{}
	for rows.Next() {
		var it ReviewItem
		var raw string
		if err := rows.Scan(&it.ID, &it.Kind, &it.Title, &it.Year, &raw); err != nil {
			return nil, fmt.Errorf("metadata review: %w", err)
		}
		if err := json.Unmarshal([]byte(raw), &it.Candidates); err != nil {
			return nil, fmt.Errorf("metadata review: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("metadata review: %w", err)
	}
	return out, nil
}

// titleByID reads a title of the library.
func (s *Syncer) titleByID(ctx context.Context, id string) (title, error) {
	t := title{id: id}
	err := s.db.QueryRowContext(ctx, `
		SELECT 'movie', title, COALESCE(year, 0), norm_title || '|' || COALESCE(year, 0) FROM movies WHERE id = ?1
		UNION ALL
		SELECT 'series', title, COALESCE(year, 0), norm_title || '|' || COALESCE(year, 0) FROM series WHERE id = ?1`, id).
		Scan(&t.ownerType, &t.name, &t.year, &t.lookupKey)
	if errors.Is(err, sql.ErrNoRows) {
		return t, fmt.Errorf("title %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return t, fmt.Errorf("title %s: %w", id, err)
	}
	return t, nil
}

// Choose is the user's answer in Revisar: this TMDB id is the title.
func (s *Syncer) Choose(ctx context.Context, ownerID string, tmdbID int) error {
	if tmdbID <= 0 {
		return fmt.Errorf("%w: tmdb_id must be positive", ErrInvalid)
	}
	t, err := s.titleByID(ctx, ownerID)
	if err != nil {
		return err
	}
	enabled, key, err := s.settings(ctx)
	if err != nil {
		return err
	}
	if !enabled || key == "" {
		return fmt.Errorf("%w: TMDB is off", ErrInvalid)
	}
	d, err := s.client(key).details(ctx, t.kind(), tmdbID)
	if err != nil {
		return fmt.Errorf("details of %d: %w", tmdbID, err)
	}
	return s.saveMatch(ctx, t, tmdbID, d)
}

// Skip is the user's answer in Revisar: it is none of these, stop asking.
func (s *Syncer) Skip(ctx context.Context, ownerID string) error {
	t, err := s.titleByID(ctx, ownerID)
	if err != nil {
		return err
	}
	return s.replaceAsUnmatched(ctx, t, "skipped", nil)
}
