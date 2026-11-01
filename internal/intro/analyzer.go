package intro

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	headMax    = 10 * time.Minute // openings are near the start…
	headShare  = 30               // …but never past 30% of the episode
	tailLen    = 4 * time.Minute  // credits are in the last minutes
	minIntro   = 15 * time.Second
	maxIntro   = 3 * time.Minute // some openings are long ("From": ~2 min)
	minCredits = 15 * time.Second
	fpTimeout  = 3 * time.Minute
	// version is part of the fingerprint key: bump it when the windows or the
	// matching change, and every episode is analyzed again.
	version = "1"
)

// Analyzer fingerprints episodes in the background, one FFmpeg at a time
// (the disk is shared with playback), and stores where their opening and
// credits are (table media_markers).
type Analyzer struct {
	db       *sql.DB
	mediaDir string // absolute MEDIA_DIR; paths come from the scanner
	ffmpeg   string
	log      *slog.Logger
	wake     chan struct{}
}

// NewAnalyzer prepares the analyzer; Run must run for its lifetime.
func NewAnalyzer(db *sql.DB, mediaDir, ffmpeg string, logger *slog.Logger) *Analyzer {
	return &Analyzer{db: db, mediaDir: mediaDir, ffmpeg: ffmpeg, log: logger, wake: make(chan struct{}, 1)}
}

// Trigger asks for an analysis (after a scan); it never blocks.
func (a *Analyzer) Trigger() {
	select {
	case a.wake <- struct{}{}:
	default: // one is already queued
	}
}

// Run analyzes on every Trigger until ctx ends.
func (a *Analyzer) Run(ctx context.Context) {
	a.Trigger() // episodes added while the server was off
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.wake:
			if err := a.Analyze(ctx); err != nil && ctx.Err() == nil {
				a.log.Warn("intro analysis failed", "err", err)
			}
		}
	}
}

type episode struct {
	id, rel, series, key, storedKey string
	dur                             time.Duration
}

// Analyze fingerprints the new and changed episodes, then finds the
// openings and credits of their series again.
func (a *Analyzer) Analyze(ctx context.Context) error {
	todo, err := a.pending(ctx)
	if err != nil {
		return err
	}
	dirty := map[string]bool{}
	for _, e := range todo {
		if err := a.fingerprint(ctx, e); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			a.log.Warn("fingerprint failed", "file", e.rel, "err", err)
		}
		dirty[e.series] = true
	}
	for series := range dirty {
		if err := a.markSeries(ctx, series); err != nil {
			return err
		}
	}
	// A file that stopped being an episode (corrected in Revisar) has nothing to skip.
	if _, err := a.db.ExecContext(ctx,
		`DELETE FROM media_markers WHERE media_file_id NOT IN (SELECT media_file_id FROM episodes)`); err != nil {
		return fmt.Errorf("clean markers: %w", err)
	}
	return nil
}

// pending lists the episodes that are new or changed since their fingerprint.
func (a *Analyzer) pending(ctx context.Context) ([]episode, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT e.media_file_id, mf.rel_path, e.series_id, mf.size || ':' || mf.mtime_ns || ':' || ?,
		       COALESCE(f.file_key, ''), COALESCE(mf.duration_ms, 0)
		FROM episodes e
		JOIN media_files mf ON mf.id = e.media_file_id
		LEFT JOIN fingerprints f ON f.media_file_id = e.media_file_id
		WHERE mf.status IN ('ok', 'review')`, version)
	if err != nil {
		return nil, fmt.Errorf("list episodes: %w", err)
	}
	defer rows.Close()
	var todo []episode
	for rows.Next() {
		var e episode
		var ms int64
		if err := rows.Scan(&e.id, &e.rel, &e.series, &e.key, &e.storedKey, &ms); err != nil {
			return nil, fmt.Errorf("list episodes: %w", err)
		}
		if e.key != e.storedKey && ms > 0 {
			e.dur = time.Duration(ms) * time.Millisecond
			todo = append(todo, e)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list episodes: %w", err)
	}
	return todo, nil
}

// fingerprint stores the start and end fingerprints of e. A file that
// cannot be read (no audio) is stored empty, so it is not retried until it
// changes.
func (a *Analyzer) fingerprint(ctx context.Context, e episode) error {
	src := filepath.Join(a.mediaDir, filepath.FromSlash(e.rel))
	tailStart := max(0, e.dur-tailLen)
	head, err := a.chromaprint(ctx, src, 0, min(headMax, e.dur*headShare/100))
	var tail []byte
	if err == nil {
		tail, err = a.chromaprint(ctx, src, tailStart, e.dur-tailStart)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if head == nil || tail == nil { // stored empty: never NULL
		head, tail = []byte{}, []byte{}
	}
	if _, dbErr := a.db.ExecContext(ctx, `
		INSERT INTO fingerprints (media_file_id, file_key, head, tail, tail_start_ms) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (media_file_id) DO UPDATE SET file_key = excluded.file_key, head = excluded.head,
			tail = excluded.tail, tail_start_ms = excluded.tail_start_ms`,
		e.id, e.key, head, tail, tailStart.Milliseconds()); dbErr != nil {
		return fmt.Errorf("save fingerprint: %w", dbErr)
	}
	return err
}

// chromaprint returns the raw fingerprint (little-endian uint32 points) of
// the first audio track between from and from+length.
func (a *Analyzer) chromaprint(ctx context.Context, src string, from, length time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, fpTimeout)
	defer cancel()
	secs := func(d time.Duration) string { return strconv.FormatFloat(d.Seconds(), 'f', 3, 64) }
	//nolint:gosec // the binary comes from the operator's config (FFMPEG_PATH); src is a file: path inside MEDIA_DIR
	cmd := exec.CommandContext(ctx, a.ffmpeg, "-hide_banner", "-nostdin", "-loglevel", "error",
		"-ss", secs(from), "-t", secs(length), "-i", "file:"+src,
		"-map", "0:a:0", "-ac", "1", "-f", "chromaprint", "-fp_format", "raw", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		return nil, fmt.Errorf("chromaprint %s: %w: %s", src, err, msg[:min(len(msg), 200)])
	}
	return out, nil
}

func points(raw []byte) []uint32 {
	out := make([]uint32, len(raw)/4)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(raw[4*i:])
	}
	return out
}

type epPrint struct {
	id, season string
	head, tail []uint32
	tailStart  time.Duration
}

// markSeries finds each episode's opening and credits by comparing it with
// the other episodes of its season (of the series, when it is alone).
func (a *Analyzer) markSeries(ctx context.Context, series string) error {
	eps, err := a.prints(ctx, series)
	if err != nil {
		return err
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("markers of %s: %w", series, err)
	}
	defer func() { _ = tx.Rollback() }()
	found := 0
	for _, e := range eps {
		partners := sameSeason(eps, e)
		var intro, credits Span
		for _, p := range partners {
			if s, _, ok := Match(e.head, p.head, minIntro, maxIntro); ok && s.Len() > intro.Len() {
				intro = s
			}
			if s, _, ok := Match(e.tail, p.tail, minCredits, tailLen); ok && s.Len() > credits.Len() {
				credits = Span{e.tailStart + s.Start, e.tailStart + s.End}
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_markers WHERE media_file_id = ?`, e.id); err != nil {
			return fmt.Errorf("markers of %s: %w", e.id, err)
		}
		for kind, s := range map[string]Span{"intro": intro, "credits": credits} {
			if s.Len() > 0 {
				found++
				if _, err := tx.ExecContext(ctx,
					`INSERT INTO media_markers (media_file_id, kind, start_ms, end_ms) VALUES (?, ?, ?, ?)`,
					e.id, kind, s.Start.Milliseconds(), s.End.Milliseconds()); err != nil {
					return fmt.Errorf("markers of %s: %w", e.id, err)
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("markers of %s: %w", series, err)
	}
	a.log.Info("openings and credits", "series", series, "episodes", len(eps), "markers", found)
	return nil
}

// prints loads the fingerprints of a series' episodes.
func (a *Analyzer) prints(ctx context.Context, series string) ([]epPrint, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT e.media_file_id, e.season_id, f.head, f.tail, f.tail_start_ms
		FROM episodes e JOIN fingerprints f ON f.media_file_id = e.media_file_id
		WHERE e.series_id = ? ORDER BY e.media_file_id`, series)
	if err != nil {
		return nil, fmt.Errorf("fingerprints of %s: %w", series, err)
	}
	defer rows.Close()
	var eps []epPrint
	for rows.Next() {
		var p epPrint
		var head, tail []byte
		var ms int64
		if err := rows.Scan(&p.id, &p.season, &head, &tail, &ms); err != nil {
			return nil, fmt.Errorf("fingerprints of %s: %w", series, err)
		}
		p.head, p.tail, p.tailStart = points(head), points(tail), time.Duration(ms)*time.Millisecond
		eps = append(eps, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("fingerprints of %s: %w", series, err)
	}
	return eps, nil
}

// sameSeason returns the other episodes of e's season, or of the series
// when e is the only one in its season.
func sameSeason(eps []epPrint, e epPrint) []epPrint {
	var season, others []epPrint
	for _, p := range eps {
		if p.id == e.id {
			continue
		}
		others = append(others, p)
		if p.season == e.season {
			season = append(season, p)
		}
	}
	if len(season) > 0 {
		return season
	}
	return others
}
