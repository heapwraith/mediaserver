package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Progress rules.
const (
	// MinResumeMs is the least position that counts as "started": below it the
	// item does not show in "Continuar assistindo".
	MinResumeMs = 30_000
	// homeRowLimit is the size of each profile row of the home page.
	homeRowLimit = 20
	// homeSeriesLimit caps the series looked at for "Próximos episódios".
	homeSeriesLimit = 40
)

// IsWatched reports whether a position counts as watched (90% or more).
func IsWatched(positionMs, durationMs int64) bool {
	return durationMs > 0 && positionMs*10 >= durationMs*9
}

// Progress is where a profile stopped in one media file.
type Progress struct {
	MediaID    string `json:"media_id"`
	PositionMs int64  `json:"position_ms"`
	DurationMs int64  `json:"duration_ms"`
	Watched    bool   `json:"watched"`
	UpdatedAt  int64  `json:"updated_at,omitzero"`
}

// SaveProgress stores the position of a profile in a media file. The file's
// probed duration wins over the client's. Once watched (90%), an item stays
// watched until it is unmarked by hand.
func (s Store) SaveProgress(ctx context.Context, profileID, mediaID string, positionMs, durationMs int64) (Progress, error) {
	if positionMs < 0 || durationMs < 0 {
		return Progress{}, fmt.Errorf("%w: position and duration must be >= 0", ErrInvalid)
	}
	var known int64
	err := s.DB.QueryRowContext(ctx,
		`SELECT COALESCE(duration_ms, 0) FROM media_files WHERE id = ? AND status IN ('ok', 'review')`, mediaID).Scan(&known)
	if errors.Is(err, sql.ErrNoRows) {
		return Progress{}, fmt.Errorf("media %s: %w", mediaID, ErrNotFound)
	}
	if err != nil {
		return Progress{}, fmt.Errorf("media %s: %w", mediaID, err)
	}
	if known > 0 {
		durationMs = known
	}
	if durationMs > 0 {
		positionMs = min(positionMs, durationMs)
	}
	p := Progress{MediaID: mediaID}
	err = s.DB.QueryRowContext(ctx,
		`INSERT INTO progress (profile_id, media_file_id, position_ms, duration_ms, watched, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (profile_id, media_file_id) DO UPDATE SET
		     position_ms = excluded.position_ms, duration_ms = excluded.duration_ms,
		     watched = MAX(progress.watched, excluded.watched), updated_at = excluded.updated_at
		 RETURNING position_ms, duration_ms, watched, updated_at`,
		profileID, mediaID, positionMs, durationMs, IsWatched(positionMs, durationMs), time.Now().UnixMilli()).
		Scan(&p.PositionMs, &p.DurationMs, &p.Watched, &p.UpdatedAt)
	if err != nil {
		return Progress{}, fmt.Errorf("save progress %s: %w", mediaID, err)
	}
	return p, nil
}

// ProgressOf returns the progress of a profile in a media file (zero when
// it never played it).
func (s Store) ProgressOf(ctx context.Context, profileID, mediaID string) (Progress, error) {
	p := Progress{MediaID: mediaID}
	err := s.DB.QueryRowContext(ctx,
		`SELECT position_ms, duration_ms, watched, updated_at FROM progress WHERE profile_id = ? AND media_file_id = ?`,
		profileID, mediaID).Scan(&p.PositionMs, &p.DurationMs, &p.Watched, &p.UpdatedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return p, fmt.Errorf("progress %s: %w", mediaID, err)
	}
	return p, nil
}

// WatchedTarget selects what SetWatched marks: media files, a season or a
// whole series (exactly one of them).
type WatchedTarget struct {
	MediaIDs []string `json:"media_ids,omitempty"`
	SeasonID string   `json:"season_id,omitempty"`
	SeriesID string   `json:"series_id,omitempty"`
	Watched  bool     `json:"watched"`
}

// SetWatched marks (position back to 0) or unmarks (progress forgotten) the
// target as watched for a profile.
func (s Store) SetWatched(ctx context.Context, profileID string, t WatchedTarget) error {
	set := 0
	for _, ok := range []bool{len(t.MediaIDs) > 0, t.SeasonID != "", t.SeriesID != ""} {
		if ok {
			set++
		}
	}
	if set != 1 {
		return fmt.Errorf("%w: give exactly one of media_ids, season_id or series_id", ErrInvalid)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set watched: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	ids := t.MediaIDs
	if len(ids) == 0 {
		col, id := "season_id", t.SeasonID
		if t.SeriesID != "" {
			col, id = "series_id", t.SeriesID
		}
		//nolint:gosec // col is one of two fixed column names
		rows, err := tx.QueryContext(ctx, `SELECT media_file_id FROM episodes WHERE `+col+` = ?`, id)
		if err != nil {
			return fmt.Errorf("episodes of %s: %w", id, err)
		}
		if err := scanAll(rows, func(r *sql.Rows) error {
			var m string
			err := r.Scan(&m)
			ids = append(ids, m)
			return err
		}); err != nil {
			return err
		}
		if len(ids) == 0 {
			return fmt.Errorf("%s %s: %w", col, id, ErrNotFound)
		}
	}

	now := time.Now().UnixMilli()
	for _, id := range ids {
		var res sql.Result
		if t.Watched {
			res, err = tx.ExecContext(ctx,
				`INSERT INTO progress (profile_id, media_file_id, position_ms, duration_ms, watched, updated_at)
				 SELECT ?, id, 0, COALESCE(duration_ms, 0), 1, ? FROM media_files WHERE id = ?
				 ON CONFLICT (profile_id, media_file_id) DO UPDATE SET
				     position_ms = 0, watched = 1, updated_at = excluded.updated_at`, profileID, now, id)
		} else {
			// Unknown ids are not an error here: there is nothing to forget.
			_, err = tx.ExecContext(ctx, `DELETE FROM progress WHERE profile_id = ? AND media_file_id = ?`, profileID, id)
		}
		if err != nil {
			return fmt.Errorf("set watched %s: %w", id, err)
		}
		if res != nil {
			if n, _ := res.RowsAffected(); n == 0 {
				return fmt.Errorf("media %s: %w", id, ErrNotFound)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("set watched: %w", err)
	}
	return nil
}

// episodeOrder is the viewing order of a series (specials last), shared by
// SeriesByID and the "next episode" logic.
const episodeOrder = `se.number = 0, se.number, e.number, e.absolute_number, mf.rel_path`

type epRef struct {
	mediaID string
	special bool
}

// episodesInOrder lists the media ids of a series' episodes in viewing order.
func (s Store) episodesInOrder(ctx context.Context, seriesID string) ([]epRef, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT e.media_file_id, se.number = 0
		 FROM episodes e
		 JOIN seasons se ON se.id = e.season_id
		 JOIN media_files mf ON mf.id = e.media_file_id
		 WHERE e.series_id = ?
		 ORDER BY `+episodeOrder, seriesID)
	if err != nil {
		return nil, fmt.Errorf("episodes of %s: %w", seriesID, err)
	}
	var out []epRef
	err = scanAll(rows, func(r *sql.Rows) error {
		var e epRef
		err := r.Scan(&e.mediaID, &e.special)
		out = append(out, e)
		return err
	})
	return out, err
}

// after returns the episode that follows index i: specials only follow
// specials, so a series does not run into its extras episodes.
func after(order []epRef, i int, skip func(string) bool) string {
	for j := i + 1; j < len(order); j++ {
		if (order[j].special && !order[i].special) || skip(order[j].mediaID) {
			continue
		}
		return order[j].mediaID
	}
	return ""
}

// upNext picks the episode to watch next: the last one touched while it is
// not finished, else the first unwatched one after it, else the first
// unwatched (non-special) one of the series. "" when everything was watched
// or nothing was touched yet.
func upNext(order []epRef, prog map[string]Progress) string {
	last := -1
	for i, e := range order {
		p, ok := prog[e.mediaID]
		if ok && (last < 0 || p.UpdatedAt > prog[order[last].mediaID].UpdatedAt) {
			last = i
		}
	}
	if last < 0 {
		return ""
	}
	if !prog[order[last].mediaID].Watched {
		return order[last].mediaID
	}
	watched := func(id string) bool { return prog[id].Watched }
	if next := after(order, last, watched); next != "" {
		return next
	}
	for _, e := range order {
		if !e.special && !watched(e.mediaID) {
			return e.mediaID
		}
	}
	return ""
}

// SeriesProgress is a profile's progress in every episode of a series.
type SeriesProgress struct {
	// Episodes maps media ids to their progress (only the ones played).
	Episodes map[string]Progress `json:"episodes"`
	// UpNext is the media id of the episode to play next ("" = start at the
	// first one, or everything was watched).
	UpNext string `json:"up_next,omitzero"`
}

func (s Store) seriesProgressMap(ctx context.Context, profileID, seriesID string) (map[string]Progress, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT p.media_file_id, p.position_ms, p.duration_ms, p.watched, p.updated_at
		 FROM episodes e
		 JOIN progress p ON p.media_file_id = e.media_file_id AND p.profile_id = ?
		 WHERE e.series_id = ?`, profileID, seriesID)
	if err != nil {
		return nil, fmt.Errorf("progress of %s: %w", seriesID, err)
	}
	out := map[string]Progress{}
	err = scanAll(rows, func(r *sql.Rows) error {
		var p Progress
		err := r.Scan(&p.MediaID, &p.PositionMs, &p.DurationMs, &p.Watched, &p.UpdatedAt)
		out[p.MediaID] = p
		return err
	})
	return out, err
}

// SeriesProgressOf returns a profile's progress in a series.
func (s Store) SeriesProgressOf(ctx context.Context, profileID, seriesID string) (SeriesProgress, error) {
	var one int
	if err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM series WHERE id = ?`, seriesID).Scan(&one); errors.Is(err, sql.ErrNoRows) {
		return SeriesProgress{}, fmt.Errorf("series %s: %w", seriesID, ErrNotFound)
	} else if err != nil {
		return SeriesProgress{}, fmt.Errorf("series %s: %w", seriesID, err)
	}
	prog, err := s.seriesProgressMap(ctx, profileID, seriesID)
	if err != nil {
		return SeriesProgress{}, err
	}
	out := SeriesProgress{Episodes: prog}
	if len(prog) > 0 {
		order, err := s.episodesInOrder(ctx, seriesID)
		if err != nil {
			return out, err
		}
		out.UpNext = upNext(order, prog)
	}
	return out, nil
}

// HomeItem is a card of the profile rows of the home page, or the next
// episode of the player.
type HomeItem struct {
	MediaID string `json:"media_id"`
	Kind    string `json:"kind"` // movie | episode
	// Title is the movie or series title.
	Title string `json:"title"`
	// ParentID is the movie or series id (the details page).
	ParentID   string   `json:"parent_id"`
	SeriesKind string   `json:"series_kind,omitzero"` // series | anime, for episodes
	Season     int      `json:"season,omitzero"`
	Episode    *Episode `json:"episode,omitempty"`
	PositionMs int64    `json:"position_ms"`
	DurationMs int64    `json:"duration_ms"`
	UpdatedAt  int64    `json:"updated_at,omitzero"`
	Color      string   `json:"color,omitzero"`
}

// Home holds the profile rows of the home page.
type Home struct {
	Continue []HomeItem `json:"continue"`
	Next     []HomeItem `json:"next"`
	// Again lists every title the profile finished, latest first: the home
	// page moves them to the end of its rows.
	Again []Finished `json:"again"`
}

// Finished is a movie, or a series with every regular episode, the profile
// watched ("Assista novamente").
type Finished struct {
	Kind  string `json:"kind"` // movie | series | anime
	ID    string `json:"id"`   // the movie or series (the details page)
	Title string `json:"title"`
	Color string `json:"color,omitzero"`
	At    int64  `json:"at"` // when it was last watched
}

// homeItemCols and homeItemJoins read the movie or episode card of the
// media_file_id column of a row source aliased x.
const homeItemCols = `COALESCE(mv.id, ''), COALESCE(mv.title, ''), COALESCE(s.id, ''), COALESCE(s.title, ''),
        COALESCE(s.kind, ''), COALESCE(se.number, 0), COALESCE(e.id, ''), COALESCE(e.number, 0),
        COALESCE(e.number_end, 0), COALESCE(e.absolute_number, 0), COALESCE(e.title, ''),
        COALESCE(mf.duration_ms, 0), COALESCE(mf.width, 0), COALESCE(mf.height, 0), e.added_at,
        COALESCE(ic.color, '')`

const homeItemJoins = `
	JOIN media_files mf ON mf.id = x.media_file_id
	LEFT JOIN movies mv ON mv.media_file_id = x.media_file_id
	LEFT JOIN episodes e ON e.media_file_id = x.media_file_id
	LEFT JOIN series s ON s.id = e.series_id
	LEFT JOIN seasons se ON se.id = e.season_id
	LEFT JOIN image_cache ic ON ic.owner_id = COALESCE(mv.id, e.id)`

// scanHomeItem reads homeItemCols (after the caller's own leading columns).
func scanHomeItem(dest []any, it *HomeItem, scan func(...any) error) error {
	var movieID, movieTitle, seriesID, seriesTitle, epID, epTitle string
	var number, numberEnd, absolute int
	var duration int64
	var width, height int
	var added sql.NullInt64
	dest = append(dest, &movieID, &movieTitle, &seriesID, &seriesTitle, &it.SeriesKind, &it.Season, &epID,
		&number, &numberEnd, &absolute, &epTitle, &duration, &width, &height, &added, &it.Color)
	if err := scan(dest...); err != nil {
		return err
	}
	if movieID != "" {
		it.Kind, it.Title, it.ParentID, it.SeriesKind = "movie", movieTitle, movieID, ""
	} else {
		it.Kind, it.Title, it.ParentID = "episode", seriesTitle, seriesID
		it.Episode = &Episode{
			ID: epID, Number: number, NumberEnd: numberEnd, Absolute: absolute, Title: epTitle,
			MediaID: it.MediaID, DurationMs: duration, Width: width, Height: height, AddedAt: added.Int64, Color: it.Color,
		}
	}
	if it.DurationMs == 0 {
		it.DurationMs = duration
	}
	return nil
}

// HomeOf returns the "Continuar assistindo" and "Próximos episódios" rows of
// a profile.
func (s Store) HomeOf(ctx context.Context, profileID string) (Home, error) {
	h := Home{Continue: []HomeItem{}, Next: []HomeItem{}, Again: []Finished{}}
	// Most recent first; a series shows only its latest episode.
	rows, err := s.DB.QueryContext(ctx,
		`SELECT x.media_file_id, x.position_ms, x.duration_ms, x.updated_at, `+homeItemCols+`
		 FROM (SELECT media_file_id, position_ms, duration_ms, updated_at FROM progress
		       WHERE profile_id = ? AND watched = 0 AND position_ms >= ?
		       ORDER BY updated_at DESC LIMIT ?) x`+homeItemJoins+`
		 WHERE mv.id IS NOT NULL OR e.id IS NOT NULL
		 ORDER BY x.updated_at DESC`, profileID, MinResumeMs, homeRowLimit*5)
	if err != nil {
		return h, fmt.Errorf("continue watching: %w", err)
	}
	inContinue := map[string]bool{}
	err = scanAll(rows, func(r *sql.Rows) error {
		var it HomeItem
		if err := scanHomeItem([]any{&it.MediaID, &it.PositionMs, &it.DurationMs, &it.UpdatedAt}, &it, r.Scan); err != nil {
			return err
		}
		if inContinue[it.ParentID] || len(h.Continue) >= homeRowLimit {
			return nil
		}
		inContinue[it.ParentID] = true
		h.Continue = append(h.Continue, it)
		return nil
	})
	if err != nil {
		return h, err
	}

	// Series with recent activity whose last touched episode was finished.
	rows, err = s.DB.QueryContext(ctx,
		`SELECT e.series_id, MAX(p.updated_at) AS at
		 FROM progress p JOIN episodes e ON e.media_file_id = p.media_file_id
		 WHERE p.profile_id = ?
		 GROUP BY e.series_id ORDER BY at DESC LIMIT ?`, profileID, homeSeriesLimit)
	if err != nil {
		return h, fmt.Errorf("next episodes: %w", err)
	}
	var series []string
	if err := scanAll(rows, func(r *sql.Rows) error {
		var id string
		var at int64
		err := r.Scan(&id, &at)
		series = append(series, id)
		return err
	}); err != nil {
		return h, err
	}
	for _, id := range series {
		if len(h.Next) >= homeRowLimit {
			break
		}
		if inContinue[id] {
			continue
		}
		next, err := s.nextForSeries(ctx, profileID, id)
		if err != nil {
			return h, err
		}
		if next == "" {
			continue
		}
		it, err := s.homeItem(ctx, next)
		if err != nil {
			return h, err
		}
		h.Next = append(h.Next, it)
	}

	// Watched movies, and series whose regular episodes (not season 0) were all watched.
	rows, err = s.DB.QueryContext(ctx,
		`SELECT 'movie', mv.id, mv.title, COALESCE(ic.color, ''), p.updated_at
		 FROM progress p
		 JOIN movies mv ON mv.media_file_id = p.media_file_id
		 LEFT JOIN image_cache ic ON ic.owner_id = mv.id
		 WHERE p.profile_id = ?1 AND p.watched = 1
		 UNION ALL
		 SELECT s.kind, s.id, s.title, COALESCE(ic.color, ''), MAX(p.updated_at)
		 FROM progress p
		 JOIN episodes e ON e.media_file_id = p.media_file_id
		 JOIN series s ON s.id = e.series_id
		 LEFT JOIN image_cache ic ON ic.owner_id = s.id
		 WHERE p.profile_id = ?1 AND p.watched = 1
		 GROUP BY s.id
		 HAVING NOT EXISTS (
		     SELECT 1 FROM episodes e2
		     JOIN seasons se ON se.id = e2.season_id
		     LEFT JOIN progress p2 ON p2.media_file_id = e2.media_file_id AND p2.profile_id = ?1
		     WHERE e2.series_id = s.id AND se.number <> 0 AND COALESCE(p2.watched, 0) = 0)
		 ORDER BY 5 DESC`, profileID)
	if err != nil {
		return h, fmt.Errorf("watched titles: %w", err)
	}
	err = scanAll(rows, func(r *sql.Rows) error {
		var f Finished
		err := r.Scan(&f.Kind, &f.ID, &f.Title, &f.Color, &f.At)
		h.Again = append(h.Again, f)
		return err
	})
	return h, err
}

// nextForSeries is the up-next episode of a series when the last one touched
// was finished ("" otherwise: it is still in progress, or all is watched).
func (s Store) nextForSeries(ctx context.Context, profileID, seriesID string) (string, error) {
	prog, err := s.seriesProgressMap(ctx, profileID, seriesID)
	if err != nil {
		return "", err
	}
	order, err := s.episodesInOrder(ctx, seriesID)
	if err != nil {
		return "", err
	}
	next := upNext(order, prog)
	if p, ok := prog[next]; ok && !p.Watched {
		return "", nil // still in progress: that is "Continuar assistindo"
	}
	return next, nil
}

// homeItem reads the card of one media file (no progress).
func (s Store) homeItem(ctx context.Context, mediaID string) (HomeItem, error) {
	it := HomeItem{MediaID: mediaID}
	row := s.DB.QueryRowContext(ctx,
		`SELECT `+homeItemCols+` FROM (SELECT ? AS media_file_id) x`+homeItemJoins, mediaID)
	err := scanHomeItem(nil, &it, row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return it, fmt.Errorf("media %s: %w", mediaID, ErrNotFound)
	}
	if err != nil {
		return it, fmt.Errorf("media %s: %w", mediaID, err)
	}
	return it, nil
}

// NextEpisode returns the episode after a media file in its series, for the
// player's autoplay. ErrNotFound when the file is not an episode or is the
// last one.
func (s Store) NextEpisode(ctx context.Context, mediaID string) (HomeItem, error) {
	var seriesID string
	err := s.DB.QueryRowContext(ctx, `SELECT series_id FROM episodes WHERE media_file_id = ?`, mediaID).Scan(&seriesID)
	if errors.Is(err, sql.ErrNoRows) {
		return HomeItem{}, fmt.Errorf("episode %s: %w", mediaID, ErrNotFound)
	}
	if err != nil {
		return HomeItem{}, fmt.Errorf("episode %s: %w", mediaID, err)
	}
	order, err := s.episodesInOrder(ctx, seriesID)
	if err != nil {
		return HomeItem{}, err
	}
	for i, e := range order {
		if e.mediaID != mediaID {
			continue
		}
		if next := after(order, i, func(string) bool { return false }); next != "" {
			return s.homeItem(ctx, next)
		}
		break
	}
	return HomeItem{}, fmt.Errorf("next of %s: %w", mediaID, ErrNotFound)
}
