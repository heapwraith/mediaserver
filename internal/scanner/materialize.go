package scanner

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json/v2"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"
	"uuid"
)

// fileRow is a media_files row as materialize needs it.
type fileRow struct {
	id      string
	rel     string
	size    int64
	hash    string
	addedAt int64
}

// Override is a manual correction (table overrides). Empty fields keep the
// parser's value.
type Override struct {
	Kind    string `json:"kind,omitempty"` // movie | series | anime | ignored
	Title   string `json:"title,omitempty"`
	Year    int    `json:"year,omitempty"`
	Season  *int   `json:"season,omitempty"`
	Episode *int   `json:"episode,omitempty"`
}

// item is one file after parsing, classification and overrides.
type item struct {
	file fileRow
	p    Parsed

	kind       Kind
	reason     string
	title      string // movie title, or series title
	titleFixed bool   // title comes from an override
	kindFixed  bool
	year       int
	season     int
	episode    int
	episodeEnd int
	absolute   int
	version    int
}

// Guess is what the scanner understood of a file in the review queue.
type Guess struct {
	Kind     string `json:"kind"`
	Title    string `json:"title,omitempty"`
	Year     int    `json:"year,omitempty"`
	Season   *int   `json:"season,omitempty"`
	Episode  int    `json:"episode,omitempty"`
	Absolute int    `json:"absolute,omitempty"`
}

// resolve applies the classification and the override of one file.
func resolve(f fileRow, ov *Override) *item {
	p := Parse(f.rel)
	cl := Classify(p)
	if isPromo(f.rel, f.size) {
		cl = Classification{Kind: KindIgnored}
	}
	it := &item{file: f, p: p, kind: cl.Kind, reason: cl.Reason, title: p.Title, year: p.Year, version: p.Version}
	if p.SeasonEpisode {
		it.season, it.episode, it.episodeEnd = p.Season, p.Episode, p.EpisodeEnd
	} else if p.AbsoluteEp {
		it.season, it.episode, it.absolute = 1, p.Absolute, p.Absolute
	}
	if ov == nil {
		return it
	}

	if ov.Title != "" {
		it.title, it.titleFixed = ov.Title, true
	}
	if ov.Year != 0 {
		it.year = ov.Year
	}
	switch ov.Kind {
	case "ignored":
		it.kind, it.reason, it.kindFixed = KindIgnored, "", true
	case string(KindMovie):
		it.kind, it.reason, it.kindFixed = KindMovie, "", true
	case string(KindSeries), string(KindAnime):
		it.kind, it.reason, it.kindFixed = Kind(ov.Kind), "", true
		if it.season == 0 && !p.HasSeason {
			it.season = 1
		}
	}
	if ov.Season != nil {
		it.season = *ov.Season
	}
	if ov.Episode != nil {
		it.episode = *ov.Episode
		it.episodeEnd = 0
	}
	if it.title == "" && (it.kind == KindMovie || it.kind == KindSeries || it.kind == KindAnime) {
		it.kind, it.reason = KindReview, ReasonNoTitle
	}
	if (it.kind == KindSeries || it.kind == KindAnime) && it.episode == 0 {
		it.kind, it.reason = KindReview, ReasonNoSeason
	}
	return it
}

// seriesGroup collects the episodes of one series/anime (same NormTitle).
type seriesGroup struct {
	norm      string
	id        string
	kind      Kind
	title     string
	year      int
	dir       string // folder of the series (for artwork), "" if unknown
	items     []*item
	addedAt   int64
	seasons   map[int]string // number → season id
	seasonDir map[int]string
}

// built is the library computed from the files.
type built struct {
	items   []*item
	movies  []*item
	series  []*seriesGroup
	extras  []builtExtra
	movieID map[string]string // media_file_id → movie id
	epID    map[string]string // media_file_id → episode id
}

type builtExtra struct {
	it        *item
	ownerType string
	ownerID   string
	title     string
}

// existingIDs keeps IDs stable between scans (AGENTS.md §5.6).
type existingIDs struct {
	series      map[string]string // norm_title → id
	seriesAdded map[string]int64
	seasons     map[string]string // series_id/number → id
	movies      map[string]string // media_file_id → id
	episodes    map[string]string // media_file_id → id
	extras      map[string]string // media_file_id → id
}

// materialize rebuilds movies/series/seasons/episodes/extras from the files
// and the overrides, in one transaction.
func (s *Scanner) materialize(ctx context.Context, sidecars []string) error {
	start := time.Now()
	files, err := s.loadFiles(ctx)
	if err != nil {
		return err
	}
	overrides, err := s.loadOverrides(ctx)
	if err != nil {
		return err
	}
	ids, err := s.loadIDs(ctx)
	if err != nil {
		return err
	}

	b := build(files, overrides, ids)
	if err := s.writeLibrary(ctx, b, ids, sidecars); err != nil {
		return err
	}
	s.log.Debug("library materialized", "files", len(files), "movies", len(b.movies),
		"series", len(b.series), "duration_ms", time.Since(start).Milliseconds())
	return nil
}

func (s *Scanner) loadFiles(ctx context.Context) ([]fileRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, rel_path, size, COALESCE(partial_hash, ''), added_at FROM media_files WHERE library_id = ?`, s.libraryID)
	if err != nil {
		return nil, fmt.Errorf("load files: %w", err)
	}
	defer rows.Close()
	var out []fileRow
	for rows.Next() {
		var f fileRow
		if err := rows.Scan(&f.id, &f.rel, &f.size, &f.hash, &f.addedAt); err != nil {
			return nil, fmt.Errorf("scan file row: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load files: %w", err)
	}
	slices.SortFunc(out, func(a, b fileRow) int { return strings.Compare(a.rel, b.rel) })
	return out, nil
}

// overrideSet finds the override of a file by path + size, or by size +
// partial hash when the file was renamed while the server was off.
type overrideSet struct {
	byPath map[string]*Override
	byHash map[string]*Override
}

func (o overrideSet) find(f fileRow) *Override {
	if ov := o.byPath[hashKey(f.size, f.rel)]; ov != nil {
		return ov
	}
	if f.hash != "" {
		return o.byHash[hashKey(f.size, f.hash)]
	}
	return nil
}

func (s *Scanner) loadOverrides(ctx context.Context) (overrideSet, error) {
	set := overrideSet{byPath: map[string]*Override{}, byHash: map[string]*Override{}}
	rows, err := s.db.QueryContext(ctx,
		`SELECT rel_path, size, COALESCE(partial_hash, ''), COALESCE(kind, ''), COALESCE(title, ''),
		        COALESCE(year, 0), season, episode FROM overrides`)
	if err != nil {
		return set, fmt.Errorf("load overrides: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rel, hash string
		var size int64
		var season, episode sql.NullInt64
		ov := &Override{}
		if err := rows.Scan(&rel, &size, &hash, &ov.Kind, &ov.Title, &ov.Year, &season, &episode); err != nil {
			return set, fmt.Errorf("scan override: %w", err)
		}
		if season.Valid {
			v := int(season.Int64)
			ov.Season = &v
		}
		if episode.Valid {
			v := int(episode.Int64)
			ov.Episode = &v
		}
		set.byPath[hashKey(size, rel)] = ov
		if hash != "" {
			set.byHash[hashKey(size, hash)] = ov
		}
	}
	if err := rows.Err(); err != nil {
		return set, fmt.Errorf("load overrides: %w", err)
	}
	return set, nil
}

func (s *Scanner) loadIDs(ctx context.Context) (existingIDs, error) {
	ids := existingIDs{
		series: map[string]string{}, seriesAdded: map[string]int64{}, seasons: map[string]string{},
		movies: map[string]string{}, episodes: map[string]string{}, extras: map[string]string{},
	}
	queries := []struct {
		sql  string
		scan func(*sql.Rows) error
	}{
		{`SELECT id, norm_title, added_at FROM series`, func(r *sql.Rows) error {
			var id, norm string
			var added int64
			if err := r.Scan(&id, &norm, &added); err != nil {
				return err
			}
			ids.series[norm], ids.seriesAdded[id] = id, added
			return nil
		}},
		{`SELECT id, series_id, number FROM seasons`, func(r *sql.Rows) error {
			var id, sid string
			var n int
			if err := r.Scan(&id, &sid, &n); err != nil {
				return err
			}
			ids.seasons[seasonKey(sid, n)] = id
			return nil
		}},
		{`SELECT id, media_file_id FROM movies`, scanPair(ids.movies)},
		{`SELECT id, media_file_id FROM episodes`, scanPair(ids.episodes)},
		{`SELECT id, media_file_id FROM extras`, scanPair(ids.extras)},
	}
	for _, q := range queries {
		if err := queryEach(ctx, s.db, q.sql, q.scan); err != nil {
			return ids, err
		}
	}
	return ids, nil
}

func scanPair(m map[string]string) func(*sql.Rows) error {
	return func(r *sql.Rows) error {
		var id, fileID string
		if err := r.Scan(&id, &fileID); err != nil {
			return err
		}
		m[fileID] = id
		return nil
	}
}

func queryEach(ctx context.Context, db *sql.DB, query string, f func(*sql.Rows) error) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query %q: %w", query, err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := f(rows); err != nil {
			return fmt.Errorf("scan %q: %w", query, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("query %q: %w", query, err)
	}
	return nil
}

func seasonKey(seriesID string, n int) string { return fmt.Sprintf("%s/%d", seriesID, n) }

func idOr(m map[string]string, key string) string {
	if id, ok := m[key]; ok {
		return id
	}
	return uuid.NewV7().String()
}

// build is the pure part of materialize: files + overrides → library.
func build(files []fileRow, overrides overrideSet, ids existingIDs) built {
	b := built{movieID: map[string]string{}, epID: map[string]string{}}
	groups := map[string]*seriesGroup{}
	var groupOrder []string

	for _, f := range files {
		it := resolve(f, overrides.find(f))
		b.items = append(b.items, it)
		switch it.kind {
		case KindMovie:
			b.movies = append(b.movies, it)
			b.movieID[f.id] = idOr(ids.movies, f.id)
		case KindSeries, KindAnime:
			norm := NormTitle(it.title)
			g := groups[norm]
			if g == nil {
				g = &seriesGroup{norm: norm, seasons: map[int]string{}, seasonDir: map[int]string{}}
				groups[norm] = g
				groupOrder = append(groupOrder, norm)
			}
			g.items = append(g.items, it)
		}
	}

	for _, norm := range groupOrder {
		g := groups[norm]
		finishGroup(g, ids)
		b.series = append(b.series, g)
		for _, it := range g.items {
			if it.kind == KindSeries || it.kind == KindAnime {
				b.epID[it.file.id] = idOr(ids.episodes, it.file.id)
			}
		}
	}
	b.extras = linkExtras(b)
	return b
}

// finishGroup picks the series kind, title, year and folder, and resolves
// duplicate episodes.
func finishGroup(g *seriesGroup, ids existingIDs) {
	anime, series := 0, 0
	bestScore := -1 << 31
	years := map[int]int{}
	dirs := map[string]int{}
	for _, it := range g.items {
		weight := 1
		if it.kindFixed {
			weight = 1000
		}
		if it.kind == KindAnime {
			anime += weight
		} else {
			series += weight
		}
		// display title: override > matching folder name > cleanest file name
		cands := []struct {
			t     string
			score int
		}{{it.title, titleScore(it.title)}}
		if it.titleFixed {
			cands[0].score = 1 << 20
		} else if it.p.FolderTitle != "" && NormTitle(it.p.FolderTitle) == g.norm {
			cands = append(cands, struct {
				t     string
				score int
			}{it.p.FolderTitle, titleScore(it.p.FolderTitle) + 50})
		}
		for _, c := range cands {
			if c.score > bestScore {
				g.title, bestScore = c.t, c.score
			}
		}
		if y := cmp.Or(it.year, it.p.FolderYear); y != 0 {
			years[y]++
		}
		if it.p.TitleDir != "" && NormTitle(it.p.FolderTitle) == g.norm {
			dirs[it.p.TitleDir]++
		}
	}
	g.kind = KindSeries
	if anime > series {
		g.kind = KindAnime
	}
	g.year = mostCommon(years)
	g.dir = mostCommon(dirs)
	g.id = idOr(ids.series, g.norm)
	g.addedAt = ids.seriesAdded[g.id]

	// One file per episode: the highest version wins, then the biggest file.
	best := map[string]*item{}
	for _, it := range g.items {
		it.kind = g.kind
		key := fmt.Sprintf("%d/%d", it.season, it.episode)
		cur := best[key]
		if cur == nil {
			best[key] = it
			continue
		}
		winner, loser := cur, it
		if it.version > cur.version || (it.version == cur.version && it.file.size > cur.file.size) {
			winner, loser = it, cur
		}
		best[key] = winner
		if loser.version < winner.version {
			loser.kind, loser.reason = KindIgnored, "superseded"
		} else {
			loser.kind, loser.reason = KindReview, ReasonDuplicate
		}
	}
	for _, it := range g.items {
		if it.kind != g.kind {
			continue
		}
		if _, ok := g.seasons[it.season]; !ok {
			g.seasons[it.season] = idOr(ids.seasons, seasonKey(g.id, it.season))
		}
		if it.p.SeasonFolder && g.seasonDir[it.season] == "" {
			g.seasonDir[it.season] = it.p.Dir
		}
		if g.addedAt == 0 || it.file.addedAt < g.addedAt {
			g.addedAt = it.file.addedAt
		}
	}
}

func mostCommon[K cmp.Ordered](m map[K]int) K {
	var best K
	n := 0
	for k, c := range m {
		if c > n || (c == n && k < best) {
			best, n = k, c
		}
	}
	return best
}

// linkExtras attaches trailers/extras to the movie, season or series of
// their folder. Unowned extras go to review.
func linkExtras(b built) []builtExtra {
	moviesByDir := map[string][]*item{}
	for _, m := range b.movies {
		moviesByDir[m.p.Dir] = append(moviesByDir[m.p.Dir], m)
	}
	seasonByDir := map[string]string{}
	seriesByDir := map[string]string{}
	for _, g := range b.series {
		for n, dir := range g.seasonDir {
			seasonByDir[dir] = g.seasons[n]
		}
		for _, it := range g.items {
			if it.kind != g.kind {
				continue
			}
			stop := it.p.TitleDir
			for d := it.p.Dir; d != ""; d = parentDir(d) {
				seriesByDir[d] = g.id
				if d == stop || stop == "" {
					break
				}
			}
		}
		if g.dir != "" {
			seriesByDir[g.dir] = g.id
		}
	}

	var out []builtExtra
	for _, it := range b.items {
		if it.kind != KindTrailer && it.kind != KindExtra {
			continue
		}
		base := strings.TrimSuffix(path.Base(it.file.rel), path.Ext(it.file.rel))
		e := builtExtra{it: it, title: cleanTitle(base)}
		dir := it.p.OwnerDir
		movies := moviesByDir[dir]
		switch {
		case dir == "":
			// loose trailer in the root: nobody to attach it to
		case len(movies) == 1:
			e.ownerType, e.ownerID = "movie", b.movieID[movies[0].file.id]
		case len(movies) > 1:
			for _, m := range movies {
				mb := strings.TrimSuffix(path.Base(m.file.rel), path.Ext(m.file.rel))
				if strings.HasPrefix(strings.ToLower(base), strings.ToLower(mb)) {
					e.ownerType, e.ownerID = "movie", b.movieID[m.file.id]
				}
			}
		case seasonByDir[dir] != "":
			e.ownerType, e.ownerID = "season", seasonByDir[dir]
		case seriesByDir[dir] != "":
			e.ownerType, e.ownerID = "series", seriesByDir[dir]
		}
		if e.ownerID == "" {
			it.kind, it.reason = KindReview, ReasonNoOwner
			continue
		}
		out = append(out, e)
	}
	return out
}

// mediaKind maps the classification to media_files.kind / status.
func (it *item) mediaKindStatus() (kind, status string) {
	switch it.kind {
	case KindMovie:
		return "movie", "ok"
	case KindSeries, KindAnime:
		return "episode", "ok"
	case KindTrailer:
		return "trailer", "ok"
	case KindExtra:
		return "extra", "ok"
	case KindIgnored:
		return "unknown", "ignored"
	default:
		return "unknown", "review"
	}
}

func (it *item) guess() Guess {
	g := Guess{Kind: string(Classify(it.p).Kind), Title: it.p.Title, Year: it.p.Year, Absolute: it.p.Absolute}
	if g.Kind == string(KindReview) {
		switch {
		case it.p.SeasonEpisode || it.p.AbsoluteEp:
			g.Kind = string(KindSeries)
			if it.p.Group != "" || it.p.Hint == HintAnime {
				g.Kind = string(KindAnime)
			}
		default:
			g.Kind = string(KindMovie)
		}
	}
	if it.p.HasSeason {
		s := it.p.Season
		g.Season = &s
	}
	g.Episode = cmp.Or(it.p.Episode, it.p.Absolute)
	return g
}

// writeLibrary replaces the materialized tables in one transaction, reusing
// the IDs computed by build.
func (s *Scanner) writeLibrary(ctx context.Context, b built, ids existingIDs, sidecars []string) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin materialize: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, q := range []string{
		`DELETE FROM extras`, `DELETE FROM episodes`, `DELETE FROM seasons`, `DELETE FROM movies`,
		`DELETE FROM subtitles WHERE source = 'external'`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("materialize %q: %w", q, err)
		}
	}

	images := imagesByDir(sidecars)
	now := time.Now().UnixMilli()

	// media_files: kind, status, review info, per-file thumbnail
	upd, err := tx.PrepareContext(ctx,
		`UPDATE media_files SET kind = ?, status = ?, review_reason = ?, guess_json = ?, image_path = ? WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("prepare media update: %w", err)
	}
	defer upd.Close()
	for _, it := range b.items {
		kind, status := it.mediaKindStatus()
		var reason, guess any
		if status != "ok" {
			reason = nullStr(it.reason)
		}
		if status == "review" {
			data, err := json.Marshal(it.guess())
			if err != nil {
				return fmt.Errorf("encode guess: %w", err)
			}
			guess = string(data)
		}
		base := strings.TrimSuffix(path.Base(it.file.rel), path.Ext(it.file.rel))
		thumb := cmp.Or(images[it.p.Dir][strings.ToLower(base)+"-thumb"], images[it.p.Dir][strings.ToLower(base)])
		if _, err := upd.ExecContext(ctx, kind, status, reason, guess, nullStr(thumb), it.file.id); err != nil {
			return fmt.Errorf("update %s: %w", it.file.rel, err)
		}
	}

	// movies
	moviesPerDir := map[string]int{}
	for _, m := range b.movies {
		moviesPerDir[m.p.Dir]++
	}
	for _, m := range b.movies {
		base := strings.TrimSuffix(path.Base(m.file.rel), path.Ext(m.file.rel))
		var art Artwork
		if moviesPerDir[m.p.Dir] == 1 && m.p.Dir != "" {
			art = folderArt(images[m.p.Dir], base)
		} else {
			art = folderArt(onlyPrefixed(images[m.p.Dir], base), base)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO movies (id, media_file_id, title, sort_title, norm_title, year, poster_path, fanart_path, added_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			b.movieID[m.file.id], m.file.id, m.title, sortTitle(m.title), NormTitle(m.title), nullInt(m.year),
			nullStr(art.Poster), nullStr(art.Fanart), m.file.addedAt); err != nil {
			return fmt.Errorf("insert movie %s: %w", m.file.rel, err)
		}
	}

	// series, seasons, episodes
	keep := make([]string, 0, len(b.series))
	epStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO episodes (id, series_id, season_id, media_file_id, number, number_end, absolute_number, version, title, added_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare episode insert: %w", err)
	}
	defer epStmt.Close()
	for _, g := range b.series {
		keep = append(keep, g.id)
		var art Artwork
		if g.dir != "" {
			art = folderArt(images[g.dir], "")
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO series (id, kind, title, sort_title, norm_title, year, poster_path, fanart_path, added_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (id) DO UPDATE SET kind = excluded.kind, title = excluded.title, sort_title = excluded.sort_title,
			   norm_title = excluded.norm_title, year = excluded.year, poster_path = excluded.poster_path,
			   fanart_path = excluded.fanart_path, updated_at = excluded.updated_at`,
			g.id, string(g.kind), g.title, sortTitle(g.title), g.norm, nullInt(g.year),
			nullStr(art.Poster), nullStr(art.Fanart), g.addedAt, now); err != nil {
			return fmt.Errorf("upsert series %s: %w", g.title, err)
		}
		numbers := make([]int, 0, len(g.seasons))
		for n := range g.seasons {
			numbers = append(numbers, n)
		}
		slices.Sort(numbers)
		for _, n := range numbers {
			poster := ""
			if d := g.seasonDir[n]; d != "" {
				poster = folderArt(images[d], "").Poster
			}
			if poster == "" && g.dir != "" {
				poster = seasonArt(images[g.dir], n)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO seasons (id, series_id, number, poster_path) VALUES (?, ?, ?, ?)`,
				g.seasons[n], g.id, n, nullStr(poster)); err != nil {
				return fmt.Errorf("insert season %s/%d: %w", g.title, n, err)
			}
		}
		for _, it := range g.items {
			if it.kind != g.kind {
				continue
			}
			if _, err := epStmt.ExecContext(ctx,
				b.epID[it.file.id], g.id, g.seasons[it.season], it.file.id, it.episode, nullInt(it.episodeEnd),
				nullInt(it.absolute), nullInt(it.version), nullStr(it.p.EpisodeTitle), it.file.addedAt); err != nil {
				return fmt.Errorf("insert episode %s: %w", it.file.rel, err)
			}
		}
	}
	if err := deleteSeriesExcept(ctx, tx, keep); err != nil {
		return err
	}

	// extras
	for _, e := range b.extras {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO extras (id, media_file_id, owner_type, owner_id, extra_type, title) VALUES (?, ?, ?, ?, ?, ?)`,
			idOr(ids.extras, e.it.file.id), e.it.file.id, e.ownerType, e.ownerID, e.it.p.ExtraType, e.title); err != nil {
			return fmt.Errorf("insert extra %s: %w", e.it.file.rel, err)
		}
	}

	// external subtitles
	refs := make([]videoRef, 0, len(b.items))
	for _, it := range b.items {
		dir, file := splitRel(it.file.rel)
		refs = append(refs, videoRef{id: it.file.id, rel: it.file.rel, dir: dir, base: strings.TrimSuffix(file, path.Ext(file))})
	}
	for fileID, subs := range matchSubtitles(refs, sidecars) {
		for _, sub := range subs {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO subtitles (id, media_file_id, source, rel_path, language, codec, title, is_forced, is_default)
				 VALUES (?, ?, 'external', ?, ?, ?, ?, ?, ?)`,
				uuid.NewV7().String(), fileID, sub.RelPath, nullStr(sub.Language), sub.Codec, nullStr(sub.Title),
				boolInt(sub.Forced), boolInt(sub.Default)); err != nil {
				return fmt.Errorf("insert subtitle %s: %w", sub.RelPath, err)
			}
		}
	}

	if err := s.setState(ctx, tx, "materialize_version", materializeVersion); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit materialize: %w", err)
	}
	return nil
}

func deleteSeriesExcept(ctx context.Context, tx *sql.Tx, keep []string) error {
	all, err := seriesIDs(ctx, tx)
	if err != nil {
		return err
	}
	keepSet := make(map[string]bool, len(keep))
	for _, id := range keep {
		keepSet[id] = true
	}
	for _, id := range all {
		if keepSet[id] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM series WHERE id = ?`, id); err != nil {
			return fmt.Errorf("delete series %s: %w", id, err)
		}
	}
	return nil
}

func seriesIDs(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM series`)
	if err != nil {
		return nil, fmt.Errorf("list series: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan series id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list series: %w", err)
	}
	return ids, nil
}

// onlyPrefixed keeps "<base>-poster" style images (for folders with several movies).
func onlyPrefixed(images map[string]string, base string) map[string]string {
	out := map[string]string{}
	prefix := strings.ToLower(base) + "-"
	for k, v := range images {
		if strings.HasPrefix(k, prefix) {
			out[k] = v
		}
	}
	return out
}

// sortTitle drops leading articles: "The Matrix" sorts as "matrix".
func sortTitle(t string) string {
	n := NormTitle(t)
	for _, a := range []string{"the ", "a ", "an ", "o ", "os ", "as ", "el ", "la ", "los ", "las ", "le ", "les "} {
		if rest, ok := strings.CutPrefix(n, a); ok && rest != "" {
			return rest
		}
	}
	return n
}

func nullInt(n int) any {
	if n == 0 {
		return nil
	}
	return n
}
