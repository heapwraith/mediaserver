package stream

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mediaserver/internal/library"
)

// HLS transcoding (ADR 006): one FFmpeg session per (file, plan, start
// point) writes an EVENT playlist of fMP4 segments in DATA_DIR/transcode/<key>.
// The player adds the session start to the video time and asks for a new
// session when it seeks past what is ready.
//
// shortcut: FFmpeg runs at full speed until the end of the file (no
// throttling) and a server killed by force leaves its FFmpeg children
// running (no Job Object / Pdeathsig); add both if they become a problem.

const (
	transcodeVersion = "1" // part of the cache key: bump it when the FFmpeg arguments change
	segmentSeconds   = "4"
	playlistName     = "index.m3u8"
	idleTimeout      = 20 * time.Second // no playlist/segment request: FFmpeg is stopped
	reapEvery        = 5 * time.Second
	playlistWait     = 15 * time.Second // for the first segment of a new session
	maxStderr        = 4 << 10
)

var (
	// ErrBusy is returned when MAX_TRANSCODES video encodes are running.
	ErrBusy = errors.New("too many transcodes running")
	// ErrClosed is returned after the server started shutting down.
	ErrClosed = errors.New("transcoder closed")

	keyPattern  = regexp.MustCompile(`^[0-9a-f]{32}$`)
	filePattern = regexp.MustCompile(`^(?:index\.m3u8|init\.mp4|seg\d{1,6}\.m4s)$`)
)

// PlayRequest is the body of POST /api/media/{id}/playback.
type PlayRequest struct {
	Caps     []string `json:"caps"`     // what the client plays (see Decide)
	Audio    int      `json:"audio"`    // ffprobe index of the audio track; another value = default
	Start    float64  `json:"start"`    // seconds
	Subtitle string   `json:"subtitle"` // id of the subtitle shown ("" = none)
	// Burn is set by the API when the subtitle must be drawn into the video.
	Burn *Burn `json:"-"`
}

// Burn draws an ASS subtitle into the video, for clients that cannot run
// JASSUB. FFmpeg runs in Dir (which holds fonts/), so File needs no escaping.
type Burn struct {
	Dir  string
	File string
}

// Playback tells the client what to load.
type Playback struct {
	Direct bool   `json:"direct"`
	URL    string `json:"url"`
	// Start is the media time (s) of the stream's time 0 (HLS), or where the
	// client seeks to (direct play).
	Start  float64 `json:"start"`
	Video  string  `json:"video,omitzero"`  // HLS: copy | h264
	Audio  string  `json:"audio,omitzero"`  // HLS: copy | aac
	Burned bool    `json:"burned,omitzero"` // the subtitle is drawn into the video
}

// TranscoderOptions configure NewTranscoder.
type TranscoderOptions struct {
	Root          *os.Root // MEDIA_DIR
	FFmpeg        string
	FFprobe       string
	Dir           string // DATA_DIR/transcode
	HWAccel       string // config HWACCEL
	MaxTranscodes int    // concurrent video encodes (remuxes are not limited)
	CacheBytes    int64
	Logger        *slog.Logger
}

// Transcoder runs the HLS sessions.
type Transcoder struct {
	root    *os.Root
	rootDir string
	ffmpeg  string
	ffprobe string
	dir     string
	cache   int64
	idle    time.Duration
	log     *slog.Logger
	encoder func() encoder
	slots   chan struct{}

	mu       sync.Mutex
	sessions map[string]*session
	closed   bool
	wg       sync.WaitGroup
}

// SessionInfo describes a running FFmpeg session (the status page).
type SessionInfo struct {
	MediaID string    `json:"media_id"`
	Video   string    `json:"video"` // copy | h264
	Audio   string    `json:"audio"` // copy | aac
	Burned  bool      `json:"burned"`
	Encode  bool      `json:"encode"` // converts the video (holds a slot)
	Started time.Time `json:"started"`
	// LastRequest is when a player last asked for this session.
	LastRequest time.Time `json:"last_request,omitzero"`
}

type session struct {
	info    SessionInfo
	key     string
	dir     string
	encode  bool // holds a slot
	cancel  context.CancelFunc
	done    chan struct{}
	stopped atomic.Bool  // killed on purpose (idle, client, shutdown)
	last    atomic.Int64 // unix ns of the last request
}

func (s *session) touch() { s.last.Store(time.Now().UnixNano()) }

func (s *session) stop() {
	s.stopped.Store(true)
	s.cancel()
}

// NewTranscoder prepares the transcoder; Run must run for its lifetime.
// ctx bounds the GPU detection.
func NewTranscoder(ctx context.Context, o TranscoderOptions) (*Transcoder, error) {
	rootDir, err := filepath.Abs(o.Root.Name())
	if err != nil {
		return nil, fmt.Errorf("transcoder root: %w", err)
	}
	if err := os.MkdirAll(o.Dir, 0o750); err != nil {
		return nil, fmt.Errorf("transcode dir: %w", err)
	}
	t := &Transcoder{
		root: o.Root, rootDir: rootDir, ffmpeg: o.FFmpeg, ffprobe: o.FFprobe, dir: o.Dir,
		cache: o.CacheBytes, idle: idleTimeout, log: o.Logger,
		slots: make(chan struct{}, max(1, o.MaxTranscodes)), sessions: map[string]*session{},
	}
	t.encoder = sync.OnceValue(func() encoder {
		return detectEncoder(ctx, o.FFmpeg, o.HWAccel, o.Logger)
	})
	return t, nil
}

// Run detects the GPU encoder, cleans the cache and stops idle sessions
// until ctx ends; then it stops every FFmpeg and waits for them.
func (t *Transcoder) Run(ctx context.Context) {
	go t.encoder() // hardware detection at startup, without delaying it
	t.prune()
	tick := time.NewTicker(reapEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			t.mu.Lock()
			t.closed = true
			for _, s := range t.sessions {
				s.stop()
			}
			t.mu.Unlock()
			t.wg.Wait()
			return
		case now := <-tick.C:
			t.reap(now)
		}
	}
}

// reap stops the sessions nobody asked anything for idle time (a closed tab).
func (t *Transcoder) reap(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range t.sessions {
		if now.Sub(time.Unix(0, s.last.Load())) > t.idle {
			s.stop()
		}
	}
}

// Start decides how the client plays m (rel: its path in MEDIA_DIR) and,
// for HLS, starts or reuses the FFmpeg session.
func (t *Transcoder) Start(ctx context.Context, m library.Media, rel string, req PlayRequest) (Playback, error) {
	start := max(0, req.Start)
	if m.DurationMs > 0 {
		start = min(start, max(0, float64(m.DurationMs)/1000-1))
	}
	plan := Decide(m, req.Caps, req.Audio, req.Burn != nil)
	workDir := ""
	if req.Burn != nil {
		plan.Burn, workDir = req.Burn.File, req.Burn.Dir
	}
	// os.Root refuses anything outside MEDIA_DIR (symlinks included).
	// Checked for direct play too: the player then says the file is gone.
	info, err := t.root.Stat(filepath.FromSlash(rel))
	if errors.Is(err, fs.ErrNotExist) {
		return Playback{}, fmt.Errorf("play %s: %w: %w", m.ID, ErrNotFound, ErrFileMissing)
	}
	if err != nil || !info.Mode().IsRegular() {
		return Playback{}, fmt.Errorf("play %s: %w", m.ID, ErrNotFound)
	}
	if plan.Direct {
		return Playback{Direct: true, URL: "/api/media/" + m.ID + "/stream", Start: start}, nil
	}
	src := filepath.Join(t.rootDir, filepath.FromSlash(rel))
	start = math.Floor(start) // whole seconds: more cache hits
	if plan.Video == "copy" && start > 0 {
		start = t.keyframe(ctx, src, start)
	}
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%s|%d|%d|%+v|%.3f",
		transcodeVersion, m.ID, rel, info.Size(), info.ModTime().UnixNano(), plan, start))
	key := hex.EncodeToString(sum[:16])
	// Built before taking the lock: the first call may wait for the GPU detection.
	args := t.args(src, plan, start, filepath.Join(t.dir, key))
	sinfo := SessionInfo{MediaID: m.ID, Video: plan.Video, Audio: plan.Audio, Burned: plan.Burn != "", Encode: plan.Video != "copy"}
	if err := t.ensure(ctx, key, args, workDir, sinfo); err != nil {
		return Playback{}, err
	}
	return Playback{
		URL: "/api/transcode/" + key + "/" + playlistName, Start: start,
		Video: plan.Video, Audio: plan.Audio, Burned: plan.Burn != "",
	}, nil
}

// keyframe returns where a video stream copy seeking to at really starts:
// the keyframe FFmpeg lands on (at when ffprobe cannot tell).
func (t *Transcoder) keyframe(ctx context.Context, src string, at float64) float64 {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	//nolint:gosec // the binary comes from the operator's config (FFPROBE_PATH); src is a file: path inside MEDIA_DIR
	out, err := exec.CommandContext(ctx, t.ffprobe, "-v", "error",
		"-read_intervals", strconv.FormatFloat(at, 'f', 3, 64)+"%+#32", "-select_streams", "v:0",
		"-show_entries", "packet=pts_time,flags", "-of", "csv=p=0", "file:"+src).Output()
	if err != nil {
		return at
	}
	for line := range strings.Lines(string(out)) {
		pts, flags, _ := strings.Cut(strings.TrimSpace(line), ",")
		if v, err := strconv.ParseFloat(pts, 64); err == nil && strings.HasPrefix(flags, "K") {
			return v
		}
	}
	return at
}

// ensure reuses the running or complete session key, or starts FFmpeg.
// workDir is FFmpeg's working directory ("" = the server's).
func (t *Transcoder) ensure(ctx context.Context, key string, args []string, workDir string, info SessionInfo) error {
	encode := info.Encode
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return ErrClosed
	}
	if s := t.sessions[key]; s != nil {
		s.touch()
		return nil
	}
	dir := filepath.Join(t.dir, key)
	if complete(dir) {
		now := time.Now()
		_ = os.Chtimes(dir, now, now) // recently used: last to be pruned
		return nil
	}
	if encode {
		select {
		case t.slots <- struct{}{}:
		default:
			return ErrBusy
		}
	}
	release := func() {
		if encode {
			<-t.slots
		}
	}
	// A leftover of a stopped session (Windows may refuse to delete a file
	// that was being served).
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		release()
		return fmt.Errorf("transcode dir: %w", err)
	}
	// The session outlives the request that started it.
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	//nolint:gosec // the binary comes from the operator's config (FFMPEG_PATH); src is a file: path inside MEDIA_DIR
	cmd := exec.CommandContext(ctx, t.ffmpeg, args...)
	cmd.WaitDelay = 5 * time.Second
	cmd.Dir = workDir
	stderr := &capped{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cancel()
		release()
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	info.Started = time.Now()
	s := &session{info: info, key: key, dir: dir, encode: encode, cancel: cancel, done: make(chan struct{})}
	s.touch()
	t.sessions[key] = s
	t.log.Info("transcode started", "key", key, "encode", encode)
	t.wg.Go(func() { t.wait(s, cmd, stderr) })
	return nil
}

// wait cleans up after FFmpeg exits: an incomplete output is deleted while
// the session still owns its key, so a new session cannot lose its files.
func (t *Transcoder) wait(s *session, cmd *exec.Cmd, stderr *capped) {
	err := cmd.Wait()
	ok := complete(s.dir)
	if !ok {
		_ = os.RemoveAll(s.dir)
	}
	t.mu.Lock()
	delete(t.sessions, s.key)
	t.mu.Unlock()
	if s.encode {
		<-t.slots
	}
	s.cancel()
	close(s.done)
	switch {
	case s.stopped.Load():
		t.log.Info("transcode stopped", "key", s.key)
	case err != nil || !ok:
		t.log.Warn("transcode failed", "key", s.key, "err", err, "ffmpeg", strings.TrimSpace(stderr.String()))
	default:
		t.log.Info("transcode finished", "key", s.key)
	}
	t.prune()
}

// args builds the FFmpeg command line of a session.
func (t *Transcoder) args(src string, p Plan, start float64, dir string) []string {
	a := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	var enc encoder
	if p.Video != "copy" {
		enc = t.encoder()
		a = append(a, enc.input...)
	}
	if start > 0 {
		a = append(a, "-ss", strconv.FormatFloat(start, 'f', 3, 64))
	}
	a = append(a, "-i", "file:"+src, "-map", "0:v:0?")
	if p.AudioIndex >= 0 {
		a = append(a, "-map", "0:"+strconv.Itoa(p.AudioIndex))
	}
	a = append(a, "-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn")
	if p.Video == "copy" {
		a = append(a, "-c:v", "copy")
		if p.VideoCodec == "hevc" {
			a = append(a, "-tag:v", "hvc1") // Safari only plays hvc1
		}
	} else {
		vf := "scale=-2:'min(ih,1080)',"
		if p.Burn != "" {
			// The input seek restarts timestamps at 0: shift them back to
			// the subtitle's timeline while it is drawn.
			ss := strconv.FormatFloat(start, 'f', 3, 64)
			vf += "setpts=PTS+" + ss + "/TB,subtitles=f=" + p.Burn + ":fontsdir=fonts,setpts=PTS-STARTPTS,"
		}
		a = append(a, "-vf", vf+enc.format)
		a = append(a, enc.args...)
		a = append(a, "-profile:v", "high", "-force_key_frames", "expr:gte(t,n_forced*"+segmentSeconds+")")
	}
	switch p.Audio {
	case "copy":
		a = append(a, "-c:a", "copy")
	case "aac":
		a = append(a, "-c:a", "aac", "-ac", "2", "-b:a", "192k")
	}
	// Forward slashes: with backslashes the HLS muxer writes init.mp4 to the working directory.
	dir = filepath.ToSlash(dir)
	return append(a, "-max_muxing_queue_size", "4096",
		"-f", "hls", "-hls_time", segmentSeconds, "-hls_playlist_type", "event",
		"-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", "init.mp4",
		"-hls_segment_filename", dir+"/seg%d.m4s",
		"-hls_flags", "independent_segments", dir+"/"+playlistName)
}

// Stop ends the session key (the client left or seeks elsewhere) and waits
// for FFmpeg to exit, so its slot is free for the next session.
func (t *Transcoder) Stop(key string) {
	t.mu.Lock()
	s := t.sessions[key]
	t.mu.Unlock()
	if s != nil {
		s.stop()
		<-s.done
	}
}

// ServeFile serves the playlist or a segment of session key.
func (t *Transcoder) ServeFile(w http.ResponseWriter, r *http.Request, key, name string) error {
	if !keyPattern.MatchString(key) || !filePattern.MatchString(name) {
		return fmt.Errorf("transcode file %s/%s: %w", key, name, ErrNotFound)
	}
	t.mu.Lock()
	s := t.sessions[key]
	t.mu.Unlock()
	dir := filepath.Join(t.dir, key)
	if s != nil {
		s.touch()
	} else if !complete(dir) {
		// Stopped before the end: the player starts a new session.
		return fmt.Errorf("transcode %s: %w", key, ErrNotFound)
	}
	if name == playlistName {
		return servePlaylist(w, r, s, dir)
	}
	f, err := os.Open(filepath.Join(dir, name)) //nolint:gosec // key and name match fixed patterns
	if err != nil {
		return fmt.Errorf("transcode file %s/%s: %w", key, name, ErrNotFound)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("transcode file %s/%s: %w", key, name, err)
	}
	ctype := "video/iso.segment"
	if name == "init.mp4" {
		ctype = "video/mp4"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "", info.ModTime(), f)
	return nil
}

// servePlaylist waits for the first segment of a new session. FFmpeg
// rewrites the playlist in place (a temp file + rename fails on Windows
// while it is being read), so a torn read is read again.
func servePlaylist(w http.ResponseWriter, r *http.Request, s *session, dir string) error {
	file := filepath.Join(dir, playlistName)
	deadline := time.Now().Add(playlistWait)
	for {
		data, err := os.ReadFile(file) //nolint:gosec // dir is DATA_DIR/transcode/<hex key>
		if err == nil && bytes.HasPrefix(data, []byte("#EXTM3U")) && bytes.HasSuffix(data, []byte("\n")) &&
			bytes.Contains(data, []byte("#EXTINF")) {
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(data)
			return nil
		}
		if s == nil || time.Now().After(deadline) {
			return fmt.Errorf("playlist %s: %w", dir, ErrNotFound)
		}
		select {
		case <-r.Context().Done():
			return r.Context().Err()
		case <-s.done:
			s = nil // read once more: complete, or gone
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// complete reports whether dir holds a finished session (#EXT-X-ENDLIST).
func complete(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, playlistName)) //nolint:gosec // dir is DATA_DIR/transcode/<hex key>
	return err == nil && bytes.Contains(data, []byte("#EXT-X-ENDLIST"))
}

// prune deletes leftovers of stopped sessions and, past the cache limit,
// the least recently used complete sessions. Running sessions are kept.
func (t *Transcoder) prune() {
	t.mu.Lock()
	defer t.mu.Unlock()
	entries, err := os.ReadDir(t.dir)
	if err != nil {
		return
	}
	type cached struct {
		dir  string
		size int64
		used time.Time
	}
	var list []cached
	var total int64
	for _, e := range entries {
		if !e.IsDir() || t.sessions[e.Name()] != nil {
			continue
		}
		dir := filepath.Join(t.dir, e.Name())
		info, err := e.Info()
		if err != nil || !complete(dir) {
			_ = os.RemoveAll(dir)
			continue
		}
		size := dirSize(dir)
		list = append(list, cached{dir, size, info.ModTime()})
		total += size
	}
	slices.SortFunc(list, func(a, b cached) int { return a.used.Compare(b.used) })
	for _, c := range list {
		if total <= t.cache {
			break
		}
		if os.RemoveAll(c.dir) == nil {
			total -= c.size
		}
	}
}

func dirSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

// capped keeps the first maxStderr bytes of FFmpeg's error output (a broken
// file can print one error per frame for hours).
type capped struct{ buf []byte }

func (c *capped) Write(p []byte) (int, error) {
	c.buf = append(c.buf, p[:min(len(p), max(0, maxStderr-len(c.buf)))]...)
	return len(p), nil
}

func (c *capped) String() string { return string(c.buf) }

// Stats lists the running FFmpeg sessions, oldest first.
func (t *Transcoder) Stats() []SessionInfo {
	t.mu.Lock()
	out := make([]SessionInfo, 0, len(t.sessions))
	for _, s := range t.sessions {
		info := s.info
		if ns := s.last.Load(); ns > 0 {
			info.LastRequest = time.Unix(0, ns)
		}
		out = append(out, info)
	}
	t.mu.Unlock()
	slices.SortFunc(out, func(a, b SessionInfo) int { return a.Started.Compare(b.Started) })
	return out
}

// Slots reports the video conversions running and the limit (MAX_TRANSCODES).
func (t *Transcoder) Slots() (used, limit int) { return len(t.slots), cap(t.slots) }

// Encoder is the video encoder in use: nvenc, qsv, amf, vaapi, videotoolbox or cpu.
// It waits for the detection at startup.
func (t *Transcoder) Encoder() string { return t.encoder().Name }

// Cache is the HLS cache folder and its size limit in bytes.
func (t *Transcoder) Cache() (dir string, limit int64) { return t.dir, t.cache }
