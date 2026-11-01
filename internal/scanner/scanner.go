// Package scanner turns the media folder into a clean virtual library in the
// database. It only reads the user's files: nothing is ever renamed, moved,
// written or deleted on disk (AGENTS.md §4).
package scanner

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"uuid"

	"mediaserver/internal/probe"
)

// Prober reads technical metadata of a media file (implemented by probe.Prober).
type Prober interface {
	Probe(ctx context.Context, file string) (*probe.Result, error)
}

// Options configure a Scanner.
type Options struct {
	Root    string        // absolute media folder (MEDIA_DIR)
	Workers int           // concurrent hash/probe workers; <= 0 means NumCPU
	MinAge  time.Duration // files modified more recently are left for the next scan
	Prober  Prober        // nil disables probing
	Logger  *slog.Logger
	// AfterScan runs after each successful background scan (Start), e.g. to
	// look for openings in new episodes. It must not block.
	AfterScan func()
}

// Status is the progress/result of the current or last scan.
type Status struct {
	Running    bool      `json:"running"`
	StartedAt  time.Time `json:"started_at,omitzero"`
	FinishedAt time.Time `json:"finished_at,omitzero"`
	DurationMs int64     `json:"duration_ms"`
	Seen       int       `json:"seen"`
	Added      int       `json:"added"`
	Updated    int       `json:"updated"`
	Moved      int       `json:"moved"`
	Removed    int       `json:"removed"`
	Unchanged  int       `json:"unchanged"`
	Pending    int       `json:"pending"` // still being copied
	Review     int       `json:"review"`
	Ignored    int       `json:"ignored"`
	ProbeErrs  int       `json:"probe_errors"`
	LastError  string    `json:"last_error,omitempty"`
}

// Scanner indexes one media folder.
type Scanner struct {
	db        *sql.DB
	opt       Options
	log       *slog.Logger
	libraryID string

	run      sync.Mutex     // one scan/materialize at a time
	bg       sync.WaitGroup // background scans started by Start
	running  atomic.Bool
	again    atomic.Bool // a scan was requested while one was running
	statusMu sync.Mutex
	status   Status
}

// New prepares a scanner for opt.Root, creating the library row if needed.
func New(ctx context.Context, db *sql.DB, opt Options) (*Scanner, error) {
	if opt.Workers <= 0 {
		opt.Workers = runtime.NumCPU()
	}
	if opt.Logger == nil {
		opt.Logger = slog.New(slog.DiscardHandler)
	}
	root, err := filepath.Abs(opt.Root)
	if err != nil {
		return nil, fmt.Errorf("scanner root %s: %w", opt.Root, err)
	}
	opt.Root = root
	s := &Scanner{db: db, opt: opt, log: opt.Logger}
	if s.libraryID, err = ensureLibrary(ctx, db, root); err != nil {
		return nil, err
	}
	return s, nil
}

// ensureLibrary returns the single library, pointing it at root. Keeping the
// same row when MEDIA_DIR changes keeps IDs stable for the same relative paths.
func ensureLibrary(ctx context.Context, db *sql.DB, root string) (string, error) {
	var id, current string
	err := db.QueryRowContext(ctx, `SELECT id, root_path FROM libraries ORDER BY created_at LIMIT 1`).Scan(&id, &current)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		id = uuid.NewV7().String()
		_, err = db.ExecContext(ctx, `INSERT INTO libraries (id, name, root_path, created_at) VALUES (?, ?, ?, ?)`,
			id, filepath.Base(root), root, time.Now().UnixMilli())
		if err != nil {
			return "", fmt.Errorf("create library: %w", err)
		}
	case err != nil:
		return "", fmt.Errorf("load library: %w", err)
	case current != root:
		if _, err := db.ExecContext(ctx, `UPDATE libraries SET root_path = ?, name = ? WHERE id = ?`,
			root, filepath.Base(root), id); err != nil {
			return "", fmt.Errorf("update library root: %w", err)
		}
	}
	return id, nil
}

// Root returns the absolute media folder.
func (s *Scanner) Root() string { return s.opt.Root }

// Status returns a snapshot of the current or last scan.
func (s *Scanner) Status() Status {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	return s.status
}

func (s *Scanner) setStatus(f func(*Status)) {
	s.statusMu.Lock()
	f(&s.status)
	s.statusMu.Unlock()
}

// Start runs a scan in the background. If one is already running, another
// one is queued to run right after it (changes seen meanwhile are not lost).
// It reports whether a new scan was started now.
func (s *Scanner) Start(ctx context.Context) bool {
	if !s.running.CompareAndSwap(false, true) {
		s.again.Store(true)
		return false
	}
	s.bg.Go(func() {
		defer s.running.Store(false)
		for {
			if _, err := s.Scan(ctx); err != nil && ctx.Err() == nil {
				s.log.Error("scan failed", "err", err)
			} else if err == nil && s.opt.AfterScan != nil {
				s.opt.AfterScan()
			}
			if ctx.Err() != nil || !s.again.CompareAndSwap(true, false) {
				return
			}
		}
	})
	return true
}

// Wait blocks until background scans started by Start have returned
// (cancel their context first to stop them early).
func (s *Scanner) Wait() { s.bg.Wait() }

// existingFile is a media_files row as the scanner needs it.
type existingFile struct {
	id    string
	rel   string
	size  int64
	mtime int64
	hash  string
}

// processed is the result of hashing and probing one file.
type processed struct {
	entry    fileEntry
	hash     string
	probe    *probe.Result
	probeErr error
	err      error // the file could not be read at all
}

// Scan runs an incremental scan and blocks until it ends. Unchanged files
// (same path, size and mtime) are not opened.
func (s *Scanner) Scan(ctx context.Context) (Status, error) {
	s.run.Lock()
	defer s.run.Unlock()

	start := time.Now()
	st := Status{Running: true, StartedAt: start}
	s.setStatus(func(cur *Status) { *cur = st })

	err := s.scan(ctx, &st)

	st.Running = false
	st.FinishedAt = time.Now()
	st.DurationMs = st.FinishedAt.Sub(start).Milliseconds()
	if err != nil {
		st.LastError = err.Error()
	}
	s.setStatus(func(cur *Status) { *cur = st })
	s.log.Info("scan finished",
		"duration_ms", st.DurationMs, "seen", st.Seen, "added", st.Added, "updated", st.Updated,
		"moved", st.Moved, "removed", st.Removed, "review", st.Review, "pending", st.Pending,
		"probe_errors", st.ProbeErrs, "err", st.LastError)
	return st, err
}

func (s *Scanner) scan(ctx context.Context, st *Status) error {
	existing, err := s.loadExisting(ctx)
	if err != nil {
		return err
	}
	w, err := walk(s.opt.Root, s.opt.MinAge, time.Now(), s.log)
	if err != nil {
		return err
	}
	st.Seen, st.Pending = len(w.videos), len(w.unstable)

	seen := make(map[string]bool, len(w.videos))
	var todo []fileEntry
	for _, v := range w.videos {
		seen[v.rel] = true
		if ex, ok := existing[v.rel]; ok && ex.size == v.size && ex.mtime == v.mtime {
			st.Unchanged++
			continue
		}
		todo = append(todo, v)
	}
	// Missing = known files no longer on disk (files being copied still count as present).
	missing := map[string]existingFile{}
	for rel, ex := range existing {
		if !seen[rel] && !w.unstable[rel] {
			missing[rel] = ex
		}
	}

	if len(todo) > 0 {
		if err := s.processAll(ctx, todo, existing, missing, st); err != nil {
			return err
		}
	}
	if len(missing) > 0 {
		if err := s.removeMissing(ctx, missing); err != nil {
			return err
		}
		st.Removed = len(missing)
	}

	sidecarsChanged, err := s.syncSidecars(ctx, w.sidecars)
	if err != nil {
		return err
	}
	stale, err := s.materializeStale(ctx)
	if err != nil {
		return err
	}
	if st.Added+st.Updated+st.Moved+st.Removed > 0 || sidecarsChanged || stale {
		if err := s.materialize(ctx, w.sidecars); err != nil {
			return err
		}
	}
	return s.countQueues(ctx, st)
}

func (s *Scanner) loadExisting(ctx context.Context) (map[string]existingFile, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, rel_path, size, mtime_ns, COALESCE(partial_hash, '') FROM media_files WHERE library_id = ?`, s.libraryID)
	if err != nil {
		return nil, fmt.Errorf("load media files: %w", err)
	}
	defer rows.Close()
	out := map[string]existingFile{}
	for rows.Next() {
		var f existingFile
		if err := rows.Scan(&f.id, &f.rel, &f.size, &f.mtime, &f.hash); err != nil {
			return nil, fmt.Errorf("scan media file row: %w", err)
		}
		out[f.rel] = f
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load media files: %w", err)
	}
	return out, nil
}

// batchSize bounds how many processed files are kept in memory before they
// are written in one transaction.
const batchSize = 200

// processAll hashes and probes new/changed files with a worker pool and
// writes the results in batches. A new file whose size + partial hash matches
// a missing one is a move: the old row (and its ID) is kept.
func (s *Scanner) processAll(ctx context.Context, todo []fileEntry, existing, missing map[string]existingFile, st *Status) error {
	jobs := make(chan fileEntry)
	results := make(chan processed)
	var wg sync.WaitGroup
	for range min(s.opt.Workers, len(todo)) {
		wg.Go(func() {
			for e := range jobs {
				results <- s.processOne(ctx, e)
			}
		})
	}
	go func() {
		defer close(jobs)
		for _, e := range todo {
			select {
			case jobs <- e:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	missingByHash := map[string]existingFile{}
	for _, m := range missing {
		if m.hash != "" {
			missingByHash[hashKey(m.size, m.hash)] = m
		}
	}

	batch := make([]processed, 0, batchSize)
	var writeErr error
	flush := func() {
		if len(batch) == 0 || writeErr != nil {
			return
		}
		writeErr = s.writeBatch(ctx, batch, existing, missing, missingByHash, st)
		batch = batch[:0]
	}
	for r := range results {
		if r.err != nil {
			s.log.Warn("scan: cannot read file", "path", r.entry.rel, "err", r.err)
			continue
		}
		batch = append(batch, r)
		if len(batch) == batchSize {
			flush()
		}
	}
	flush()
	if writeErr != nil {
		return writeErr
	}
	return ctx.Err()
}

func hashKey(size int64, hash string) string { return fmt.Sprintf("%d:%s", size, hash) }

func (s *Scanner) processOne(ctx context.Context, e fileEntry) processed {
	r := processed{entry: e}
	abs := filepath.Join(s.opt.Root, filepath.FromSlash(e.rel))
	if r.hash, r.err = PartialHash(abs); r.err != nil {
		return r
	}
	if s.opt.Prober != nil && ctx.Err() == nil {
		r.probe, r.probeErr = s.opt.Prober.Probe(ctx, abs)
	}
	return r
}

func (s *Scanner) writeBatch(ctx context.Context, batch []processed, existing, missing, missingByHash map[string]existingFile, st *Status) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin scan batch: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	now := time.Now().UnixMilli()
	for _, r := range batch {
		e := r.entry
		id := ""
		switch ex, ok := existing[e.rel]; {
		case ok:
			// changed in place: same path, new size or mtime
			id = ex.id
			st.Updated++
			if _, err := tx.ExecContext(ctx,
				`UPDATE media_files SET size = ?, mtime_ns = ?, partial_hash = ?, updated_at = ? WHERE id = ?`,
				e.size, e.mtime, r.hash, now, id); err != nil {
				return fmt.Errorf("update %s: %w", e.rel, err)
			}
		default:
			if old, moved := missingByHash[hashKey(e.size, r.hash)]; moved {
				// renamed/moved: keep the row, its ID, progress and corrections
				id = old.id
				delete(missingByHash, hashKey(e.size, r.hash))
				delete(missing, old.rel)
				st.Moved++
				if _, err := tx.ExecContext(ctx,
					`UPDATE media_files SET rel_path = ?, mtime_ns = ?, updated_at = ? WHERE id = ?`,
					e.rel, e.mtime, now, id); err != nil {
					return fmt.Errorf("move %s -> %s: %w", old.rel, e.rel, err)
				}
				if _, err := tx.ExecContext(ctx,
					`UPDATE OR REPLACE overrides SET rel_path = ? WHERE rel_path = ? AND size = ?`,
					e.rel, old.rel, e.size); err != nil {
					return fmt.Errorf("move override %s: %w", old.rel, err)
				}
				s.log.Info("scan: file moved", "from", old.rel, "to", e.rel)
				continue // same content: probe data is still valid
			}
			id = uuid.NewV7().String()
			st.Added++
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO media_files (id, library_id, rel_path, size, mtime_ns, partial_hash, added_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				id, s.libraryID, e.rel, e.size, e.mtime, r.hash, now, now); err != nil {
				return fmt.Errorf("insert %s: %w", e.rel, err)
			}
		}
		if err := writeProbe(ctx, tx, id, r, now); err != nil {
			return err
		}
		if r.probeErr != nil {
			st.ProbeErrs++
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit scan batch: %w", err)
	}
	return nil
}

func writeProbe(ctx context.Context, tx *sql.Tx, id string, r processed, now int64) error {
	if r.probe == nil {
		var msg any
		if r.probeErr != nil {
			msg = r.probeErr.Error()
		}
		_, err := tx.ExecContext(ctx, `UPDATE media_files SET probe_error = ?, probed_at = ? WHERE id = ?`, msg, now, id)
		if err != nil {
			return fmt.Errorf("save probe error: %w", err)
		}
		return nil
	}
	p := r.probe
	if _, err := tx.ExecContext(ctx,
		`UPDATE media_files SET container = ?, duration_ms = ?, width = ?, height = ?, video_codec = ?,
		 audio_codec = ?, hdr = ?, bitrate = ?, probe_json = ?, probed_at = ?, probe_error = NULL,
		 audio_tracks = ?, subtitle_tracks = ? WHERE id = ?`,
		p.Container, p.DurationMs, p.Width, p.Height, nullStr(p.VideoCodec), nullStr(p.AudioCodec),
		nullStr(p.HDR), p.Bitrate, string(p.Raw), now, len(p.Audio), len(p.Subtitles), id); err != nil {
		return fmt.Errorf("save probe: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM subtitles WHERE media_file_id = ? AND source = 'embedded'`, id); err != nil {
		return fmt.Errorf("clear embedded subtitles: %w", err)
	}
	for _, t := range p.Subtitles {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO subtitles (id, media_file_id, source, stream_index, language, codec, title, is_forced, is_default)
			 VALUES (?, ?, 'embedded', ?, ?, ?, ?, ?, ?)`,
			uuid.NewV7().String(), id, t.Index, nullStr(t.Language), t.Codec, nullStr(t.Title),
			boolInt(t.Forced), boolInt(t.Default)); err != nil {
			return fmt.Errorf("save embedded subtitle: %w", err)
		}
	}
	return nil
}

func (s *Scanner) removeMissing(ctx context.Context, missing map[string]existingFile) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin remove: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, m := range missing {
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_files WHERE id = ?`, m.id); err != nil {
			return fmt.Errorf("remove %s: %w", m.rel, err)
		}
		s.log.Info("scan: file removed from library", "path", m.rel)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remove: %w", err)
	}
	return nil
}

// syncSidecars stores the sorted list of subtitle/image files and reports
// whether it changed since the last scan.
func (s *Scanner) syncSidecars(ctx context.Context, sidecars []string) (bool, error) {
	slices.Sort(sidecars)
	data, err := json.Marshal(sidecars)
	if err != nil {
		return false, fmt.Errorf("encode sidecars: %w", err)
	}
	if string(data) == "null" {
		data = []byte("[]")
	}
	prev, err := s.getState(ctx, "sidecars")
	if err != nil {
		return false, err
	}
	if prev == string(data) {
		return false, nil
	}
	return true, s.setState(ctx, s.db, "sidecars", string(data))
}

func (s *Scanner) loadSidecars(ctx context.Context) ([]string, error) {
	raw, err := s.getState(ctx, "sidecars")
	if err != nil || raw == "" {
		return nil, err
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("decode sidecars: %w", err)
	}
	return out, nil
}

// materializeVersion changes when the parser/classifier rules change, so the
// library is rebuilt once after an upgrade even if no file changed.
const materializeVersion = "1"

func (s *Scanner) materializeStale(ctx context.Context) (bool, error) {
	v, err := s.getState(ctx, "materialize_version")
	return v != materializeVersion, err
}

func (s *Scanner) getState(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM scan_state WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read scan state %s: %w", key, err)
	}
	return v, nil
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *Scanner) setState(ctx context.Context, db execer, key, value string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO scan_state (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
		key, value)
	if err != nil {
		return fmt.Errorf("write scan state %s: %w", key, err)
	}
	return nil
}

func (s *Scanner) countQueues(ctx context.Context, st *Status) error {
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(status = 'review'), 0), COALESCE(SUM(status = 'ignored'), 0)
		 FROM media_files WHERE library_id = ?`, s.libraryID).Scan(&st.Review, &st.Ignored)
	if err != nil {
		return fmt.Errorf("count review queue: %w", err)
	}
	return nil
}

// Rematerialize rebuilds the library from the files already in the database
// (used after manual corrections). It waits for a running scan to finish.
func (s *Scanner) Rematerialize(ctx context.Context) error {
	s.run.Lock()
	defer s.run.Unlock()
	sidecars, err := s.loadSidecars(ctx)
	if err != nil {
		return err
	}
	return s.materialize(ctx, sidecars)
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
