package stream

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Image variants and widths served by /api/images/{id} (AGENTS.md §7).
var (
	Variants = []string{"card", "backdrop", "blur"}
	Widths   = []int{480, 960, 1920}
)

const (
	// pipelineVersion is part of the cache key: bump it to regenerate every image.
	pipelineVersion = "1"
	jobTimeout      = 2 * time.Minute
	maxJobs         = 2 // concurrent FFmpeg image jobs
)

// Images generates and serves the 16:9 WebP images of movies, series,
// seasons, episodes and media files, cached in Dir.
//
// shortcut: images are generated on the first request and cache entries of
// removed items are never deleted; add a background warm-up and cleanup after
// the scan if the first page load or the cache size becomes a problem.
type Images struct {
	db      *sql.DB
	root    *os.Root
	rootDir string // absolute MEDIA_DIR, for FFmpeg
	ffmpeg  string
	dir     string
	log     *slog.Logger

	sem  chan struct{}
	mu   sync.Mutex
	jobs map[string]*job
}

type job struct {
	done    chan struct{}
	err     error
	waiters int
	cancel  context.CancelFunc
}

// NewImages prepares the image pipeline. dir is the cache folder (DATA_DIR/images).
func NewImages(db *sql.DB, root *os.Root, ffmpeg, dir string, logger *slog.Logger) (*Images, error) {
	rootDir, err := filepath.Abs(root.Name())
	if err != nil {
		return nil, fmt.Errorf("images root: %w", err)
	}
	return &Images{
		db: db, root: root, rootDir: rootDir, ffmpeg: ffmpeg, dir: dir, log: logger,
		sem: make(chan struct{}, maxJobs), jobs: map[string]*job{},
	}, nil
}

// sources are the candidate images of an owner (relative paths, "" = none).
type sources struct {
	fanart, poster, thumb, video string
	durationMs                   int64
}

// ServeImage writes the WebP image of owner id. It returns ErrNotFound when
// the id is unknown or no image can be produced for it.
func (im *Images) ServeImage(w http.ResponseWriter, r *http.Request, id, variant string, width int) error {
	ctx := r.Context()
	src, err := im.sources(ctx, id)
	if err != nil {
		return err
	}
	key := im.sourceKey(&src)
	keyDir := filepath.Join(im.dir, id, key)
	name := fmt.Sprintf("%s-%d.webp", variant, width)

	f, err := os.Open(filepath.Join(keyDir, name)) //nolint:gosec // id is a UUID, key a hex hash, name a fixed variant
	if errors.Is(err, os.ErrNotExist) {
		if _, statErr := os.Stat(keyDir); statErr == nil {
			// Generated before: this variant could not be produced.
			return fmt.Errorf("image %s: %w", id, ErrNotFound)
		}
		if err := im.generate(ctx, id, key, src); err != nil {
			return err
		}
		f, err = os.Open(filepath.Join(keyDir, name)) //nolint:gosec // same path as above
	}
	if err != nil {
		return fmt.Errorf("image %s: %w", id, ErrNotFound)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("image %s: %w", id, err)
	}
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("ETag", `"`+key+"-"+name+`"`)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeContent(w, r, "", info.ModTime(), f)
	return nil
}

// sources resolves the owner id (movie, series, season, episode or media
// file) to its local art and the video used for frames.
func (im *Images) sources(ctx context.Context, id string) (sources, error) {
	var s sources
	var fanart, poster, thumb, video sql.NullString
	var dur sql.NullInt64
	err := im.db.QueryRowContext(ctx, `
		SELECT o.fanart, o.poster, o.thumb, mf.rel_path, mf.duration_ms FROM (
			SELECT mv.fanart_path AS fanart, mv.poster_path AS poster, mf.image_path AS thumb, mf.id AS vid
			FROM movies mv JOIN media_files mf ON mf.id = mv.media_file_id WHERE mv.id = ?1
			UNION ALL
			SELECT s.fanart_path, s.poster_path, NULL,
			       (SELECT e.media_file_id FROM episodes e JOIN seasons se ON se.id = e.season_id
			        WHERE e.series_id = s.id ORDER BY se.number = 0, se.number, e.number LIMIT 1)
			FROM series s WHERE s.id = ?1
			UNION ALL
			SELECT s.fanart_path, COALESCE(se.poster_path, s.poster_path), NULL,
			       (SELECT e.media_file_id FROM episodes e WHERE e.season_id = se.id ORDER BY e.number LIMIT 1)
			FROM seasons se JOIN series s ON s.id = se.series_id WHERE se.id = ?1
			UNION ALL
			SELECT NULL, NULL, mf.image_path, mf.id
			FROM episodes e JOIN media_files mf ON mf.id = e.media_file_id WHERE e.id = ?1
			UNION ALL
			SELECT NULL, NULL, image_path, id FROM media_files WHERE id = ?1 AND status IN ('ok', 'review')
		) o LEFT JOIN media_files mf ON mf.id = o.vid LIMIT 1`, id).
		Scan(&fanart, &poster, &thumb, &video, &dur)
	if errors.Is(err, sql.ErrNoRows) {
		return s, fmt.Errorf("image owner %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return s, fmt.Errorf("image owner %s: %w", id, err)
	}
	return sources{fanart: fanart.String, poster: poster.String, thumb: thumb.String, video: video.String, durationMs: dur.Int64}, nil
}

// sourceKey hashes the identity (path, size, mtime) of every source. Sources
// that do not exist or escape MEDIA_DIR are cleared.
func (im *Images) sourceKey(s *sources) string {
	h := sha256.New()
	fmt.Fprintf(h, "v%s\n", pipelineVersion)
	for _, p := range []*string{&s.fanart, &s.poster, &s.thumb, &s.video} {
		if *p == "" {
			fmt.Fprintln(h, "-")
			continue
		}
		info, err := im.root.Stat(filepath.FromSlash(*p))
		if err != nil || !info.Mode().IsRegular() {
			*p = ""
			fmt.Fprintln(h, "-")
			continue
		}
		fmt.Fprintf(h, "%s|%d|%d\n", *p, info.Size(), info.ModTime().UnixNano())
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// generate runs (or joins) the job that writes every variant of id/key. The
// FFmpeg processes are killed when every waiting request is gone.
func (im *Images) generate(ctx context.Context, id, key string, src sources) error {
	k := id + "/" + key
	im.mu.Lock()
	j := im.jobs[k]
	if j == nil {
		jctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), jobTimeout)
		j = &job{done: make(chan struct{}), cancel: cancel}
		im.jobs[k] = j
		go func() {
			j.err = im.run(jctx, id, key, src)
			cancel()
			im.mu.Lock()
			delete(im.jobs, k)
			im.mu.Unlock()
			close(j.done)
		}()
	}
	j.waiters++
	im.mu.Unlock()
	defer func() {
		im.mu.Lock()
		if j.waiters--; j.waiters == 0 {
			j.cancel()
		}
		im.mu.Unlock()
	}()
	select {
	case <-j.done:
		return j.err
	case <-ctx.Done():
		return fmt.Errorf("image %s: %w", id, ctx.Err())
	}
}

// run writes the variants into a temporary folder renamed to id/key at the
// end. A permanent failure leaves an empty id/key folder so it is not retried
// until a source changes.
func (im *Images) run(ctx context.Context, id, key string, src sources) error {
	select {
	case im.sem <- struct{}{}:
		defer func() { <-im.sem }()
	case <-ctx.Done():
		return fmt.Errorf("image %s: %w", id, ctx.Err())
	}
	ownerDir := filepath.Join(im.dir, id)
	if err := os.MkdirAll(ownerDir, 0o750); err != nil {
		return fmt.Errorf("image cache: %w", err)
	}
	tmp, err := os.MkdirTemp(ownerDir, ".tmp-")
	if err != nil {
		return fmt.Errorf("image cache: %w", err)
	}
	defer os.RemoveAll(tmp)

	color, err := im.render(ctx, tmp, src)
	switch {
	case errors.Is(err, exec.ErrNotFound) || ctx.Err() != nil:
		// FFmpeg missing or request gone: try again next time.
		return fmt.Errorf("image %s: %w: %w", id, ErrNotFound, err)
	case err != nil:
		im.log.WarnContext(ctx, "image generation failed", "id", id, "err", err)
		clearDir(tmp)
	}
	final := filepath.Join(ownerDir, key)
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("image cache %s: %w", id, err)
	}
	// Drop the images of older sources.
	if entries, err := os.ReadDir(ownerDir); err == nil {
		for _, e := range entries {
			if e.Name() != key && !strings.HasPrefix(e.Name(), ".tmp-") {
				_ = os.RemoveAll(filepath.Join(ownerDir, e.Name()))
			}
		}
	}
	if color != "" {
		if _, err := im.db.ExecContext(ctx,
			`INSERT INTO image_cache (owner_id, source_key, color, updated_at) VALUES (?, ?, ?, ?)
			 ON CONFLICT (owner_id) DO UPDATE SET source_key = excluded.source_key, color = excluded.color,
			 updated_at = excluded.updated_at`, id, key, color, time.Now().UnixMilli()); err != nil {
			return fmt.Errorf("save image color %s: %w", id, err)
		}
	}
	return nil
}

func clearDir(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
}

// render writes card-W, backdrop-W and blur-W .webp files into dir and
// returns the dominant color of the card.
//
// Sources: card = fanart → poster → thumb → frame;
// backdrop/blur = fanart → frame → poster → thumb.
func (im *Images) render(ctx context.Context, dir string, src sources) (string, error) {
	abs := func(rel string) string {
		if rel == "" {
			return ""
		}
		return filepath.Join(im.rootDir, filepath.FromSlash(rel))
	}
	frame := ""
	if src.video != "" && src.fanart == "" {
		f, err := im.bestFrame(ctx, abs(src.video), src.durationMs)
		switch {
		case errors.Is(err, exec.ErrNotFound) || ctx.Err() != nil:
			return "", err
		case err != nil:
			im.log.DebugContext(ctx, "no usable frame", "video", src.video, "err", err)
		default:
			frame = filepath.Join(dir, "frame.png")
			if err := os.WriteFile(frame, f, 0o600); err != nil {
				return "", fmt.Errorf("write frame: %w", err)
			}
			defer os.Remove(frame)
		}
	}
	card := firstOf(abs(src.fanart), abs(src.poster), abs(src.thumb), frame)
	back := firstOf(abs(src.fanart), frame, abs(src.poster), abs(src.thumb))
	if card == "" {
		return "", errors.New("no image source")
	}

	const fit = "scale=1920:1080:force_original_aspect_ratio=increase,crop=1920:1080,setsar=1"
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", "file:" + card}
	graph := make([]string, 0, 12)
	if back == card {
		graph = append(graph, "[0:v]"+fit+",split=2[c][b]")
	} else {
		args = append(args, "-i", "file:"+back)
		graph = append(graph, "[0:v]"+fit+"[c]", "[1:v]"+fit+"[b]")
	}
	graph = append(graph,
		"[c]split=4[card1920][cx][cy][cz]", "[cx]scale=960:540[card960]", "[cy]scale=480:270[card480]",
		"[cz]scale=1:1:flags=area,format=rgb24[color]",
		"[b]split=3[backdrop1920][bx][by]", "[bx]scale=960:540[backdrop960]",
		"[by]scale=480:270,split=2[backdrop480][bl]",
		// Pre-rendered blur (no CSS blur on TVs): blur small, then upscale.
		"[bl]gblur=sigma=12,split=3[blur480][lx][ly]", "[lx]scale=960:540[blur960]", "[ly]scale=1920:1080[blur1920]",
	)
	args = append(args, "-filter_complex", strings.Join(graph, ";"))
	for _, v := range Variants {
		quality := "80"
		if v == "blur" {
			quality = "60"
		}
		for _, w := range Widths {
			label := v + strconv.Itoa(w)
			args = append(args, "-map", "["+label+"]", "-frames:v", "1", "-c:v", "libwebp", "-quality", quality,
				"-f", "webp", filepath.Join(dir, fmt.Sprintf("%s-%d.webp", v, w)))
		}
	}
	args = append(args, "-map", "[color]", "-frames:v", "1", "-f", "rawvideo", "pipe:1")

	out, err := im.ffmpegRun(ctx, args)
	if err != nil {
		return "", err
	}
	if len(out) < 3 {
		return "", nil
	}
	return fmt.Sprintf("#%02x%02x%02x", out[0], out[1], out[2]), nil
}

func firstOf(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// bestFrame extracts candidates at 10%, 30% and 50% of the video and returns
// the sharpest one that is not black or flat, as PNG.
func (im *Images) bestFrame(ctx context.Context, video string, durationMs int64) ([]byte, error) {
	at := []float64{0}
	if durationMs > 0 {
		d := float64(durationMs) / 1000
		at = []float64{d * 0.1, d * 0.3, d * 0.5}
	}
	var frames [][]byte
	var stats []frameStats
	for _, ts := range at {
		out, err := im.ffmpegRun(ctx, []string{
			"-hide_banner", "-loglevel", "error", "-nostdin",
			"-ss", strconv.FormatFloat(ts, 'f', 3, 64), "-i", "file:" + video, "-map", "0:V:0",
			// thumbnail picks the most representative of 24 frames; then square pixels, max 1080p.
			"-vf", "thumbnail=24,scale=trunc(iw*sar/2)*2:ih,setsar=1,scale=-2:'min(ih,1080)'",
			"-frames:v", "1", "-f", "image2pipe", "-c:v", "png", "pipe:1",
		})
		if errors.Is(err, exec.ErrNotFound) || ctx.Err() != nil {
			return nil, err
		}
		if err != nil || len(out) == 0 {
			continue
		}
		img, err := png.Decode(bytes.NewReader(out))
		if err != nil {
			continue
		}
		frames = append(frames, out)
		stats = append(stats, analyze(img))
	}
	best := pickFrame(stats)
	if best < 0 {
		return nil, errors.New("no frame extracted")
	}
	return frames[best], nil
}

func (im *Images) ffmpegRun(ctx context.Context, args []string) ([]byte, error) {
	//nolint:gosec // the binary comes from the operator's config (FFMPEG_PATH); inputs are file: paths inside MEDIA_DIR
	cmd := exec.CommandContext(ctx, im.ffmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// frameStats describe a candidate frame (luma in 0-1).
type frameStats struct {
	mean, std, sharpness float64
}

// Frames darker or flatter than this are black screens, fades or title cards.
const (
	minMean = 0.06
	minStd  = 0.03
)

// pickFrame returns the sharpest usable frame, or the most contrasted one
// when every frame is dark or flat; -1 when there is none.
func pickFrame(stats []frameStats) int {
	best, fallback := -1, -1
	for i, s := range stats {
		if s.mean >= minMean && s.std >= minStd && (best < 0 || s.sharpness > stats[best].sharpness) {
			best = i
		}
		if fallback < 0 || s.std > stats[fallback].std {
			fallback = i
		}
	}
	if best < 0 {
		return fallback
	}
	return best
}

// analyze computes the mean and deviation of the luma and the variance of
// its Laplacian (sharpness) on a grid of at most ~320 columns.
func analyze(img image.Image) frameStats {
	b := img.Bounds()
	step := max(1, b.Dx()/320)
	cols, rows := b.Dx()/step, b.Dy()/step
	if cols < 3 || rows < 3 {
		return frameStats{}
	}
	luma := make([]float64, cols*rows)
	var sum float64
	for y := range rows {
		for x := range cols {
			r, g, bl, _ := img.At(b.Min.X+x*step, b.Min.Y+y*step).RGBA()
			l := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(bl)) / 0xffff
			luma[y*cols+x] = l
			sum += l
		}
	}
	n := float64(len(luma))
	mean := sum / n
	var variance float64
	for _, l := range luma {
		variance += (l - mean) * (l - mean)
	}
	var lapSum, lapSq float64
	var lapN float64
	for y := 1; y < rows-1; y++ {
		for x := 1; x < cols-1; x++ {
			i := y*cols + x
			lap := 4*luma[i] - luma[i-1] - luma[i+1] - luma[i-cols] - luma[i+cols]
			lapSum += lap
			lapSq += lap * lap
			lapN++
		}
	}
	lapMean := lapSum / lapN
	return frameStats{mean: mean, std: math.Sqrt(variance / n), sharpness: lapSq/lapN - lapMean*lapMean}
}
