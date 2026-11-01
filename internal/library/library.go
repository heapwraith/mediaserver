// Package library reads the virtual library built by the scanner: movies,
// series, seasons, episodes, extras and technical details of media files.
package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"

	"mediaserver/internal/probe"
)

// Errors returned by the Store.
var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid input")
)

// Store runs the read queries of the library.
type Store struct {
	DB *sql.DB
}

// Page selects a slice of a sorted listing.
type Page struct {
	Sort   string // title | added | year
	Desc   bool
	Offset int
	Limit  int
	// Query filters by normalized title (scanner.NormTitle): only letters,
	// digits and spaces, so it holds no LIKE wildcard.
	Query string
}

// filter is the SQL condition (and its args) of the title search on alias.
// Without a search it is empty: a LIKE '%%' would read every row.
func (p Page) filter(alias string) (string, []any) {
	if p.Query == "" {
		return "", nil
	}
	return " AND " + alias + ".norm_title LIKE ?", []any{"%" + p.Query + "%"}
}

// MaxLimit is the largest page a listing returns.
const MaxLimit = 500

// orderBy validates the page and returns its ORDER BY clause. Columns are
// prefixed with alias; ties are broken by rowid, which every index carries,
// so title and added pages are read straight from the index.
func (p Page) orderBy(alias string) (string, error) {
	if p.Offset < 0 || p.Limit < 1 || p.Limit > MaxLimit {
		return "", fmt.Errorf("%w: offset must be >= 0 and limit 1-%d", ErrInvalid, MaxLimit)
	}
	dir := "ASC"
	if p.Desc {
		dir = "DESC"
	}
	a := alias + "."
	switch p.Sort {
	case "", "title":
		return fmt.Sprintf("%ssort_title %s, %srowid %s", a, dir, a, dir), nil
	case "added":
		return fmt.Sprintf("%sadded_at %s, %srowid %s", a, dir, a, dir), nil
	case "year":
		// Unknown years always go last.
		return fmt.Sprintf("%syear IS NULL, %syear %s, %ssort_title, %srowid", a, a, dir, a, a), nil
	default:
		return "", fmt.Errorf("%w: sort must be title, added or year", ErrInvalid)
	}
}

// List is one page of a listing.
type List[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// Movie is a movie card.
type Movie struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Year       int    `json:"year,omitzero"`
	MediaID    string `json:"media_id"`
	DurationMs int64  `json:"duration_ms,omitzero"`
	Width      int    `json:"width,omitzero"`
	Height     int    `json:"height,omitzero"`
	AddedAt    int64  `json:"added_at"`
	Color      string `json:"color,omitzero"` // dominant color of the card image, once generated
}

// Movies lists the movies.
func (s Store) Movies(ctx context.Context, p Page) (List[Movie], error) {
	order, err := p.orderBy("mv")
	if err != nil {
		return List[Movie]{}, err
	}
	out := List[Movie]{Items: []Movie{}, Offset: p.Offset, Limit: p.Limit}
	where, args := p.filter("mv")
	//nolint:gosec // where is a fixed condition (Page.filter)
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM movies mv WHERE 1=1`+where, args...).Scan(&out.Total); err != nil {
		return out, fmt.Errorf("count movies: %w", err)
	}
	// The page is picked on the index alone; only its rows are joined.
	//nolint:gosec // order comes from a fixed whitelist (Page.orderBy)
	rows, err := s.DB.QueryContext(ctx,
		`SELECT mv.id, mv.title, COALESCE(mv.year, 0), mv.media_file_id, COALESCE(mf.duration_ms, 0),
		        COALESCE(mf.width, 0), COALESCE(mf.height, 0), mv.added_at, COALESCE(ic.color, '')
		 FROM (SELECT mv.rowid AS rid FROM movies mv WHERE 1=1`+where+` ORDER BY `+order+` LIMIT ? OFFSET ?) pg
		 JOIN movies mv ON mv.rowid = pg.rid
		 JOIN media_files mf ON mf.id = mv.media_file_id
		 LEFT JOIN image_cache ic ON ic.owner_id = mv.id
		 ORDER BY `+order, append(args, p.Limit, p.Offset)...)
	if err != nil {
		return out, fmt.Errorf("list movies: %w", err)
	}
	err = scanAll(rows, func(r *sql.Rows) error {
		var m Movie
		err := r.Scan(&m.ID, &m.Title, &m.Year, &m.MediaID, &m.DurationMs, &m.Width, &m.Height, &m.AddedAt, &m.Color)
		out.Items = append(out.Items, m)
		return err
	})
	return out, err
}

// MovieByID returns one movie.
func (s Store) MovieByID(ctx context.Context, id string) (Movie, error) {
	var m Movie
	err := s.DB.QueryRowContext(ctx,
		`SELECT mv.id, mv.title, COALESCE(mv.year, 0), mv.media_file_id, COALESCE(mf.duration_ms, 0),
		        COALESCE(mf.width, 0), COALESCE(mf.height, 0), mv.added_at, COALESCE(ic.color, '')
		 FROM movies mv
		 JOIN media_files mf ON mf.id = mv.media_file_id
		 LEFT JOIN image_cache ic ON ic.owner_id = mv.id
		 WHERE mv.id = ?`, id).
		Scan(&m.ID, &m.Title, &m.Year, &m.MediaID, &m.DurationMs, &m.Width, &m.Height, &m.AddedAt, &m.Color)
	if errors.Is(err, sql.ErrNoRows) {
		return m, fmt.Errorf("movie %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return m, fmt.Errorf("movie %s: %w", id, err)
	}
	return m, nil
}

// Series is a series or anime card.
type Series struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"` // series | anime
	Title    string `json:"title"`
	Year     int    `json:"year,omitzero"`
	Seasons  int    `json:"seasons"`
	Episodes int    `json:"episodes"`
	AddedAt  int64  `json:"added_at"`
	Color    string `json:"color,omitzero"`
}

// Series lists the series (kind "series") or animes (kind "anime").
func (s Store) Series(ctx context.Context, kind string, p Page) (List[Series], error) {
	order, err := p.orderBy("s")
	if err != nil {
		return List[Series]{}, err
	}
	out := List[Series]{Items: []Series{}, Offset: p.Offset, Limit: p.Limit}
	where, args := p.filter("s")
	args = append([]any{kind}, args...)
	//nolint:gosec // where is a fixed condition (Page.filter)
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM series s WHERE s.kind = ?`+where, args...).Scan(&out.Total); err != nil {
		return out, fmt.Errorf("count series: %w", err)
	}
	//nolint:gosec // order comes from a fixed whitelist (Page.orderBy)
	rows, err := s.DB.QueryContext(ctx,
		`SELECT s.id, s.kind, s.title, COALESCE(s.year, 0),
		        (SELECT COUNT(*) FROM seasons se WHERE se.series_id = s.id),
		        (SELECT COUNT(*) FROM episodes e WHERE e.series_id = s.id),
		        s.added_at, COALESCE(ic.color, '')
		 FROM (SELECT s.rowid AS rid FROM series s WHERE s.kind = ?`+where+` ORDER BY `+order+` LIMIT ? OFFSET ?) pg
		 JOIN series s ON s.rowid = pg.rid
		 LEFT JOIN image_cache ic ON ic.owner_id = s.id
		 ORDER BY `+order, append(args, p.Limit, p.Offset)...)
	if err != nil {
		return out, fmt.Errorf("list series: %w", err)
	}
	err = scanAll(rows, func(r *sql.Rows) error {
		var it Series
		err := r.Scan(&it.ID, &it.Kind, &it.Title, &it.Year, &it.Seasons, &it.Episodes, &it.AddedAt, &it.Color)
		out.Items = append(out.Items, it)
		return err
	})
	return out, err
}

// SeriesDetail is a series with its seasons and episodes.
type SeriesDetail struct {
	Series
	Seasons []Season `json:"seasons"`
}

// Season groups the episodes of one season (number 0 = specials).
type Season struct {
	ID       string    `json:"id"`
	Number   int       `json:"number"`
	Episodes []Episode `json:"episodes"`
}

// Episode is one episode file.
type Episode struct {
	ID         string `json:"id"`
	Number     int    `json:"number,omitzero"`
	NumberEnd  int    `json:"number_end,omitzero"` // multi-episode file (S01E01-E02)
	Absolute   int    `json:"absolute_number,omitzero"`
	Title      string `json:"title,omitzero"`
	MediaID    string `json:"media_id"`
	DurationMs int64  `json:"duration_ms,omitzero"`
	Width      int    `json:"width,omitzero"`
	Height     int    `json:"height,omitzero"`
	AddedAt    int64  `json:"added_at"`
	Color      string `json:"color,omitzero"`
}

// SeriesByID returns a series with its seasons and episodes, in order.
func (s Store) SeriesByID(ctx context.Context, id string) (SeriesDetail, error) {
	var d SeriesDetail
	err := s.DB.QueryRowContext(ctx,
		`SELECT s.id, s.kind, s.title, COALESCE(s.year, 0), s.added_at, COALESCE(ic.color, '')
		 FROM series s LEFT JOIN image_cache ic ON ic.owner_id = s.id WHERE s.id = ?`, id).
		Scan(&d.ID, &d.Kind, &d.Title, &d.Year, &d.AddedAt, &d.Color)
	if errors.Is(err, sql.ErrNoRows) {
		return d, fmt.Errorf("series %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return d, fmt.Errorf("series %s: %w", id, err)
	}
	d.Seasons = []Season{}
	rows, err := s.DB.QueryContext(ctx,
		`SELECT se.id, se.number, e.id, COALESCE(e.number, 0), COALESCE(e.number_end, 0),
		        COALESCE(e.absolute_number, 0), COALESCE(e.title, ''), e.media_file_id,
		        COALESCE(mf.duration_ms, 0), COALESCE(mf.width, 0), COALESCE(mf.height, 0),
		        e.added_at, COALESCE(ic.color, '')
		 FROM seasons se
		 JOIN episodes e ON e.season_id = se.id
		 JOIN media_files mf ON mf.id = e.media_file_id
		 LEFT JOIN image_cache ic ON ic.owner_id = e.id
		 WHERE se.series_id = ?
		 ORDER BY `+episodeOrder, id)
	if err != nil {
		return d, fmt.Errorf("episodes of %s: %w", id, err)
	}
	err = scanAll(rows, func(r *sql.Rows) error {
		var seasonID string
		var number int
		var e Episode
		if err := r.Scan(&seasonID, &number, &e.ID, &e.Number, &e.NumberEnd, &e.Absolute, &e.Title,
			&e.MediaID, &e.DurationMs, &e.Width, &e.Height, &e.AddedAt, &e.Color); err != nil {
			return err
		}
		if n := len(d.Seasons); n == 0 || d.Seasons[n-1].ID != seasonID {
			d.Seasons = append(d.Seasons, Season{ID: seasonID, Number: number})
		}
		last := &d.Seasons[len(d.Seasons)-1]
		last.Episodes = append(last.Episodes, e)
		d.Episodes++
		return nil
	})
	return d, err
}

// Extra is a trailer or extra linked to a movie, series or season.
type Extra struct {
	ID         string `json:"id"`
	Type       string `json:"type"` // trailer, featurette, behind_the_scenes, deleted, interview, other
	Title      string `json:"title"`
	Season     *int   `json:"season,omitempty"` // set when linked to a season
	MediaID    string `json:"media_id"`
	DurationMs int64  `json:"duration_ms,omitzero"`
}

// MovieExtras lists the trailers and extras of a movie.
func (s Store) MovieExtras(ctx context.Context, movieID string) ([]Extra, error) {
	return s.extras(ctx, `SELECT 1 FROM movies WHERE id = ?1`,
		`e.owner_type = 'movie' AND e.owner_id = ?1`, movieID)
}

// SeriesExtras lists the trailers and extras of a series and its seasons.
func (s Store) SeriesExtras(ctx context.Context, seriesID string) ([]Extra, error) {
	return s.extras(ctx, `SELECT 1 FROM series WHERE id = ?1`,
		`(e.owner_type = 'series' AND e.owner_id = ?1) OR (e.owner_type = 'season' AND se.series_id = ?1)`, seriesID)
}

func (s Store) extras(ctx context.Context, exists, where, id string) ([]Extra, error) {
	var one int
	if err := s.DB.QueryRowContext(ctx, exists, id).Scan(&one); errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("owner %s: %w", id, ErrNotFound)
	} else if err != nil {
		return nil, fmt.Errorf("owner %s: %w", id, err)
	}
	//nolint:gosec // where is one of two fixed clauses (MovieExtras, SeriesExtras)
	rows, err := s.DB.QueryContext(ctx,
		`SELECT e.id, e.extra_type, e.title, se.number, e.media_file_id, COALESCE(mf.duration_ms, 0)
		 FROM extras e
		 JOIN media_files mf ON mf.id = e.media_file_id
		 LEFT JOIN seasons se ON e.owner_type = 'season' AND se.id = e.owner_id
		 WHERE `+where+`
		 ORDER BY e.extra_type != 'trailer', se.number, e.title`, id)
	if err != nil {
		return nil, fmt.Errorf("extras of %s: %w", id, err)
	}
	out := []Extra{}
	err = scanAll(rows, func(r *sql.Rows) error {
		var x Extra
		var season sql.NullInt64
		if err := r.Scan(&x.ID, &x.Type, &x.Title, &season, &x.MediaID, &x.DurationMs); err != nil {
			return err
		}
		if season.Valid {
			n := int(season.Int64)
			x.Season = &n
		}
		out = append(out, x)
		return nil
	})
	return out, err
}

// Media is the technical detail of one file.
type Media struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"` // movie, episode, trailer, extra, unknown
	Status     string         `json:"status"`
	FileName   string         `json:"file_name"`
	Size       int64          `json:"size"`
	Container  string         `json:"container,omitzero"`
	DurationMs int64          `json:"duration_ms,omitzero"`
	Width      int            `json:"width,omitzero"`
	Height     int            `json:"height,omitzero"`
	VideoCodec string         `json:"video_codec,omitzero"`
	VideoBits  int            `json:"video_bits,omitzero"`
	AudioCodec string         `json:"audio_codec,omitzero"`
	HDR        string         `json:"hdr,omitzero"`
	Bitrate    int64          `json:"bitrate,omitzero"`
	Audio      []probe.Track  `json:"audio"`
	Subtitles  []SubtitleInfo `json:"subtitles"`
	Markers    []Marker       `json:"markers"`
}

// Marker is where the opening ("intro") or the end credits ("credits") of
// an episode are, found by internal/intro.
type Marker struct {
	Kind    string `json:"kind"`
	StartMs int64  `json:"start_ms"`
	EndMs   int64  `json:"end_ms"`
}

// SubtitleKind is how a subtitle codec is shown: "text" (WebVTT in the
// player), "ass" (styled: JASSUB or burned in) or "" (image subtitles such as
// PGS, not shown yet).
func SubtitleKind(codec string) string {
	switch codec {
	case "srt", "subrip", "webvtt", "mov_text", "text":
		return "text"
	case "ass", "ssa":
		return "ass"
	}
	return ""
}

// SubtitleInfo is an external or embedded subtitle track.
type SubtitleInfo struct {
	ID          string `json:"id"`
	Source      string `json:"source"` // external | embedded
	Kind        string `json:"kind"`   // text | ass | "" (image subtitles: not shown yet)
	Language    string `json:"language,omitzero"`
	Codec       string `json:"codec"`
	Title       string `json:"title,omitzero"`
	Forced      bool   `json:"forced,omitzero"`
	Default     bool   `json:"default,omitzero"`
	StreamIndex *int   `json:"stream_index,omitempty"`
}

// MediaByID returns the technical details and tracks of a media file.
func (s Store) MediaByID(ctx context.Context, id string) (Media, error) {
	var m Media
	var rel string
	var probeJSON sql.NullString
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, kind, status, rel_path, size, COALESCE(container, ''), COALESCE(duration_ms, 0),
		        COALESCE(width, 0), COALESCE(height, 0), COALESCE(video_codec, ''), COALESCE(audio_codec, ''),
		        COALESCE(hdr, ''), COALESCE(bitrate, 0), probe_json
		 FROM media_files WHERE id = ? AND status IN ('ok', 'review')`, id).
		Scan(&m.ID, &m.Kind, &m.Status, &rel, &m.Size, &m.Container, &m.DurationMs, &m.Width, &m.Height,
			&m.VideoCodec, &m.AudioCodec, &m.HDR, &m.Bitrate, &probeJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return m, fmt.Errorf("media %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return m, fmt.Errorf("media %s: %w", id, err)
	}
	m.FileName = path.Base(rel)
	m.Audio = []probe.Track{}
	if probeJSON.Valid {
		if res, err := probe.Parse([]byte(probeJSON.String), path.Ext(rel)); err == nil {
			if res.Audio != nil {
				m.Audio = res.Audio
			}
			m.VideoBits = res.VideoBits
		}
	}
	m.Subtitles = []SubtitleInfo{}
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, source, COALESCE(language, ''), codec, COALESCE(title, ''), is_forced, is_default, stream_index
		 FROM subtitles WHERE media_file_id = ? ORDER BY source, stream_index, language`, id)
	if err != nil {
		return m, fmt.Errorf("subtitles of %s: %w", id, err)
	}
	err = scanAll(rows, func(r *sql.Rows) error {
		var st SubtitleInfo
		var idx sql.NullInt64
		if err := r.Scan(&st.ID, &st.Source, &st.Language, &st.Codec, &st.Title, &st.Forced, &st.Default, &idx); err != nil {
			return err
		}
		if idx.Valid {
			n := int(idx.Int64)
			st.StreamIndex = &n
		}
		st.Kind = SubtitleKind(st.Codec)
		m.Subtitles = append(m.Subtitles, st)
		return nil
	})
	if err != nil {
		return m, err
	}
	m.Markers = []Marker{}
	rows, err = s.DB.QueryContext(ctx,
		`SELECT kind, start_ms, end_ms FROM media_markers WHERE media_file_id = ? ORDER BY start_ms`, id)
	if err != nil {
		return m, fmt.Errorf("markers of %s: %w", id, err)
	}
	err = scanAll(rows, func(r *sql.Rows) error {
		var mk Marker
		err := r.Scan(&mk.Kind, &mk.StartMs, &mk.EndMs)
		m.Markers = append(m.Markers, mk)
		return err
	})
	return m, err
}

// PlayablePath returns the relative path (with '/') of a media file that can
// be streamed. Missing and ignored files are not playable.
func (s Store) PlayablePath(ctx context.Context, id string) (string, error) {
	var rel string
	err := s.DB.QueryRowContext(ctx,
		`SELECT rel_path FROM media_files WHERE id = ? AND status IN ('ok', 'review')`, id).Scan(&rel)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("media %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("media %s: %w", id, err)
	}
	return rel, nil
}

func scanAll(rows *sql.Rows, f func(*sql.Rows) error) error {
	defer rows.Close()
	for rows.Next() {
		if err := f(rows); err != nil {
			return fmt.Errorf("scan row: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows: %w", err)
	}
	return nil
}
