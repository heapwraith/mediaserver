package subtitles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"mediaserver/internal/library"
)

// Embedded subtitles and fonts are extracted in one FFmpeg pass per file
// (it reads the whole file) and cached in DATA_DIR/subtitles/<media>/<key>.
//
// shortcut: extraction runs on the first request and caches of removed files
// stay; extract right after the scan and clean up if waiting becomes a problem.

const (
	extractVersion = "1" // part of the cache key
	extractTimeout = 10 * time.Minute
	maxExternal    = 50 << 20 // a subtitle file is a few hundred KB
	doneMarker     = "done"
)

// FontPattern matches the font file names Fonts returns.
var FontPattern = regexp.MustCompile(`^\d{1,5}\.(?:ttf|otf|ttc|woff2?)$`)

// Store reads and converts the subtitles of the library.
type Store struct {
	db      *sql.DB
	root    *os.Root
	rootDir string
	ffmpeg  string
	dir     string
	log     *slog.Logger

	mu   sync.Mutex
	jobs map[string]*job
}

type job struct {
	done chan struct{}
	err  error
}

// NewStore prepares the subtitles. dir is the cache (DATA_DIR/subtitles).
func NewStore(db *sql.DB, root *os.Root, ffmpeg, dir string, logger *slog.Logger) (*Store, error) {
	rootDir, err := filepath.Abs(root.Name())
	if err != nil {
		return nil, fmt.Errorf("subtitles root: %w", err)
	}
	return &Store{db: db, root: root, rootDir: rootDir, ffmpeg: ffmpeg, dir: dir, log: logger, jobs: map[string]*job{}}, nil
}

type track struct {
	id, mediaID, source, rel, codec string
	index                           int
}

func (s *Store) track(ctx context.Context, id string) (track, error) {
	var t track
	err := s.db.QueryRowContext(ctx,
		`SELECT st.id, st.media_file_id, st.source, COALESCE(st.rel_path, ''), st.codec, COALESCE(st.stream_index, -1)
		 FROM subtitles st JOIN media_files mf ON mf.id = st.media_file_id
		 WHERE st.id = ? AND mf.status IN ('ok', 'review')`, id).
		Scan(&t.id, &t.mediaID, &t.source, &t.rel, &t.codec, &t.index)
	if errors.Is(err, sql.ErrNoRows) {
		return t, fmt.Errorf("subtitle %s: %w", id, library.ErrNotFound)
	}
	if err != nil {
		return t, fmt.Errorf("subtitle %s: %w", id, err)
	}
	return t, nil
}

// VTT returns a text subtitle as WebVTT on a timeline that starts offset
// later (the start of an HLS session).
func (s *Store) VTT(ctx context.Context, id string, offset time.Duration) ([]byte, error) {
	t, err := s.track(ctx, id)
	if err != nil {
		return nil, err
	}
	if library.SubtitleKind(t.codec) != "text" {
		return nil, fmt.Errorf("%w: subtitle %s is not a text subtitle", library.ErrInvalid, id)
	}
	text, err := s.content(ctx, t, ".srt")
	if err != nil {
		return nil, err
	}
	return WriteVTT(ParseCues(text), offset), nil
}

// ASS returns an ASS/SSA subtitle in UTF-8, as JASSUB reads it.
func (s *Store) ASS(ctx context.Context, id string) ([]byte, error) {
	t, err := s.track(ctx, id)
	if err != nil {
		return nil, err
	}
	if library.SubtitleKind(t.codec) != "ass" {
		return nil, fmt.Errorf("%w: subtitle %s is not ASS", library.ErrInvalid, id)
	}
	text, err := s.content(ctx, t, ".ass")
	if err != nil {
		return nil, err
	}
	return []byte(text), nil
}

// content reads an external file (any encoding) or an extracted track.
func (s *Store) content(ctx context.Context, t track, ext string) (string, error) {
	if t.source == "external" {
		data, err := s.readExternal(t.rel)
		if err != nil {
			return "", err
		}
		return Decode(data), nil
	}
	dir, err := s.prepare(ctx, t.mediaID)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, "s"+strconv.Itoa(t.index)+ext)) //nolint:gosec // cache dir + a number
	if err != nil {
		return "", fmt.Errorf("subtitle %s: %w", t.id, library.ErrNotFound)
	}
	return Decode(data), nil
}

// readExternal reads a sidecar file through os.Root (nothing outside MEDIA_DIR).
func (s *Store) readExternal(rel string) ([]byte, error) {
	f, err := s.root.Open(filepath.FromSlash(rel))
	if err != nil {
		return nil, fmt.Errorf("subtitle %s: %w", rel, library.ErrNotFound)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxExternal))
	if err != nil {
		return nil, fmt.Errorf("read subtitle %s: %w", rel, err)
	}
	return data, nil
}

// Fonts lists the fonts attached to media file mediaID (FontPattern names).
func (s *Store) Fonts(ctx context.Context, mediaID string) ([]string, error) {
	dir, err := s.prepare(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(dir, "fonts"))
	if err != nil {
		return nil, fmt.Errorf("fonts of %s: %w", mediaID, err)
	}
	names := []string{}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.Size() > 0 && FontPattern.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// FontPath returns the file of an attached font listed by Fonts.
func (s *Store) FontPath(ctx context.Context, mediaID, name string) (string, error) {
	if !FontPattern.MatchString(name) {
		return "", fmt.Errorf("font %s: %w", name, library.ErrNotFound)
	}
	dir, err := s.prepare(ctx, mediaID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "fonts", name), nil
}

// BurnFile prepares an ASS subtitle for FFmpeg's subtitles filter: it
// returns the folder to run FFmpeg in (it holds fonts/) and the subtitle
// file name inside it, so no path needs filter escaping.
func (s *Store) BurnFile(ctx context.Context, id string) (dir, file string, err error) {
	t, err := s.track(ctx, id)
	if err != nil {
		return "", "", err
	}
	if library.SubtitleKind(t.codec) != "ass" {
		return "", "", fmt.Errorf("%w: subtitle %s is not ASS", library.ErrInvalid, id)
	}
	if dir, err = s.prepare(ctx, t.mediaID); err != nil {
		return "", "", err
	}
	if t.source == "embedded" {
		return dir, "s" + strconv.Itoa(t.index) + ".ass", nil
	}
	data, err := s.readExternal(t.rel)
	if err != nil {
		return "", "", err
	}
	file = "x-" + t.id + ".ass" // t.id is a UUID
	if err := os.WriteFile(filepath.Join(dir, file), []byte(Decode(data)), 0o600); err != nil {
		return "", "", fmt.Errorf("write subtitle %s: %w", id, err)
	}
	return dir, file, nil
}

// media is what extraction needs from a media file.
type media struct {
	rel, probe string
	subs       []track
}

// prepare extracts the embedded subtitles and fonts of mediaID once.
func (s *Store) prepare(ctx context.Context, mediaID string) (string, error) {
	var m media
	var probeJSON sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT rel_path, probe_json FROM media_files WHERE id = ? AND status IN ('ok', 'review')`, mediaID).
		Scan(&m.rel, &probeJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("media %s: %w", mediaID, library.ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("media %s: %w", mediaID, err)
	}
	m.probe = probeJSON.String
	info, err := s.root.Stat(filepath.FromSlash(m.rel)) // os.Root: nothing outside MEDIA_DIR
	if err != nil {
		return "", fmt.Errorf("media %s: %w", mediaID, library.ErrNotFound)
	}
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%d|%d", extractVersion, m.rel, info.Size(), info.ModTime().UnixNano()))
	dir := filepath.Join(s.dir, mediaID, hex.EncodeToString(sum[:8]))
	if _, err := os.Stat(filepath.Join(dir, doneMarker)); err == nil {
		return dir, nil
	}

	s.mu.Lock()
	j := s.jobs[dir]
	if j == nil {
		// Another job may have finished between the first stat and this lock.
		if _, err := os.Stat(filepath.Join(dir, doneMarker)); err == nil {
			s.mu.Unlock()
			return dir, nil
		}
		j = &job{done: make(chan struct{})}
		s.jobs[dir] = j
		// Detached from the request: other players wait for the same job.
		jctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), extractTimeout)
		go func() {
			defer cancel()
			j.err = s.extract(jctx, mediaID, m, dir)
			s.mu.Lock()
			delete(s.jobs, dir)
			s.mu.Unlock()
			close(j.done)
		}()
	}
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-j.done:
		if j.err != nil {
			return "", j.err
		}
		return dir, nil
	}
}

// extract runs FFmpeg: every text/ASS track to s<index>.srt|.ass and every
// attached font to fonts/<index>.<ext> (never the name stored in the file).
func (s *Store) extract(ctx context.Context, mediaID string, m media, dir string) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT stream_index, codec FROM subtitles WHERE media_file_id = ? AND source = 'embedded' ORDER BY stream_index`, mediaID)
	if err != nil {
		return fmt.Errorf("subtitles of %s: %w", mediaID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var t track
		if err := rows.Scan(&t.index, &t.codec); err != nil {
			return fmt.Errorf("subtitles of %s: %w", mediaID, err)
		}
		m.subs = append(m.subs, t)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("subtitles of %s: %w", mediaID, err)
	}

	// Retry only this version; another version may still have active readers.
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(filepath.Join(dir, "fonts"), 0o750); err != nil {
		return fmt.Errorf("subtitle cache: %w", err)
	}
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}
	fonts := fontStreams(m.probe)
	for _, f := range fonts {
		args = append(args, "-dump_attachment:"+strconv.Itoa(f.index), "fonts/"+strconv.Itoa(f.index)+f.ext)
	}
	args = append(args, "-i", "file:"+filepath.Join(s.rootDir, filepath.FromSlash(m.rel)))
	outputs := 0
	for _, t := range m.subs {
		var codec, name string
		switch {
		case t.codec == "ass":
			codec, name = "copy", ".ass"
		case t.codec == "ssa":
			codec, name = "ass", ".ass"
		case library.SubtitleKind(t.codec) == "text":
			codec, name = "srt", ".srt"
		default:
			continue // image subtitles (PGS, VobSub)
		}
		idx := strconv.Itoa(t.index)
		args = append(args, "-map", "0:"+idx, "-c:s", codec, "s"+idx+name)
		outputs++
	}
	if outputs == 0 && len(fonts) == 0 {
		return markDone(dir)
	}
	if outputs == 0 {
		args = append(args, "-t", "0", "-f", "null", "-") // only the attachments
	}
	start := time.Now()
	//nolint:gosec // the binary comes from the operator's config (FFMPEG_PATH); the input is a file: path inside MEDIA_DIR
	cmd := exec.CommandContext(ctx, s.ffmpeg, args...)
	cmd.Dir = dir // outputs are relative names
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(dir)
		msg := strings.TrimSpace(stderr.String())
		return fmt.Errorf("extract subtitles of %s: %w: %s", mediaID, err, msg[:min(len(msg), 300)])
	}
	s.log.Info("subtitles extracted", "media", mediaID, "tracks", outputs, "fonts", len(fonts),
		"ms", time.Since(start).Milliseconds())
	return markDone(dir)
}

func markDone(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "fonts"), 0o750); err != nil {
		return fmt.Errorf("subtitle cache: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, doneMarker), nil, 0o600); err != nil {
		return fmt.Errorf("subtitle cache: %w", err)
	}
	return nil
}

type fontStream struct {
	index int
	ext   string
}

var fontExts = []string{".ttf", ".otf", ".ttc", ".woff", ".woff2"}

// fontStreams finds the attached fonts in the ffprobe JSON of a file.
func fontStreams(probeJSON string) []fontStream {
	var p struct {
		Streams []struct {
			Index     int               `json:"index"`
			CodecType string            `json:"codec_type"`
			CodecName string            `json:"codec_name"`
			Tags      map[string]string `json:"tags"`
		} `json:"streams"`
	}
	if probeJSON == "" || json.Unmarshal([]byte(probeJSON), &p) != nil {
		return nil
	}
	var out []fontStream
	for _, st := range p.Streams {
		if st.CodecType != "attachment" {
			continue
		}
		ext := strings.ToLower(path.Ext(st.Tags["filename"]))
		if !slices.Contains(fontExts, ext) {
			switch {
			case st.CodecName == "otf":
				ext = ".otf"
			case st.CodecName == "ttf", strings.Contains(strings.ToLower(st.Tags["mimetype"]), "font"):
				ext = ".ttf"
			default:
				continue // cover images and other attachments
			}
		}
		out = append(out, fontStream{st.Index, ext})
	}
	return out
}
