package api

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mediaserver/internal/db"
	"mediaserver/internal/stream"
	"mediaserver/internal/sysinfo"
)

// GET /api/admin/backup: a consistent copy of the database (profiles,
// progress, Revisar corrections), made while the server runs. It is restored
// from the command line with the server stopped (`mediaserver restore`).
func (d Deps) backup(w http.ResponseWriter, r *http.Request) {
	if d.DB == nil || d.DataDir == "" {
		writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "backup not available", Code: "unavailable"})
		return
	}
	file, err := db.Backup(r.Context(), d.DB, d.DataDir)
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	// Deferred first, runs last: Windows cannot remove an open file.
	defer func() {
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			d.Logger.Warn("remove backup copy", "file", file, "err", err)
		}
	}()
	f, err := os.Open(file) //nolint:gosec // our own temporary file in DATA_DIR
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	defer f.Close()
	now := time.Now()
	name := "mediaserver-" + now.Format("20060102-1504") + ".db"
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, name, now, f)
}

// statusBody is GET /api/admin/status (the "Status do servidor" page).
type statusBody struct {
	Version   string          `json:"version"`
	StartedAt time.Time       `json:"started_at"`
	UptimeS   int64           `json:"uptime_s"`
	CPU       sysinfo.CPU     `json:"cpu"`
	Memory    sysinfo.Mem     `json:"memory"`
	Encoder   string          `json:"encoder"` // "" without FFmpeg
	GPUs      []sysinfo.GPU   `json:"gpus"`    // NVIDIA only
	Sessions  []sessionStatus `json:"sessions"`
	Slots     struct {
		Used  int `json:"used"`
		Limit int `json:"limit"`
	} `json:"slots"`
	Disk    diskUse `json:"disk"`
	Library struct {
		Movies int `json:"movies"`
		Series int `json:"series"`
		Animes int `json:"animes"`
		Files  int `json:"files"`
		Review int `json:"review"`
	} `json:"library"`
}

type sessionStatus struct {
	stream.SessionInfo
	File string `json:"file"`
}

// diskUse: bytes in DATA_DIR.
type diskUse struct {
	Transcode      int64 `json:"transcode"`
	TranscodeLimit int64 `json:"transcode_limit"`
	Images         int64 `json:"images"`
	Subtitles      int64 `json:"subtitles"`
	Database       int64 `json:"database"`
}

// adminState is shared by the status requests (Deps is copied by value).
type adminState struct {
	cpu  sysinfo.Sampler
	mu   sync.Mutex
	disk diskUse
	at   time.Time
}

// diskEvery: folder sizes are walked at most this often.
const diskEvery = 10 * time.Second

// GET /api/admin/status
func (d Deps) status(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var b statusBody
	b.Version = d.Version
	b.StartedAt = d.Started
	if !d.Started.IsZero() {
		b.UptimeS = int64(time.Since(d.Started).Seconds())
	}
	b.CPU = d.admin.cpu.CPU()
	b.Memory = sysinfo.Memory()
	b.GPUs = sysinfo.NvidiaGPUs(ctx)
	b.Sessions = []sessionStatus{}
	if t := d.Transcoder; t != nil {
		b.Encoder = t.Encoder()
		b.Slots.Used, b.Slots.Limit = t.Slots()
		for _, s := range t.Stats() {
			st := sessionStatus{SessionInfo: s}
			if m, err := d.Library.MediaByID(ctx, s.MediaID); err == nil {
				st.File = m.FileName
			}
			b.Sessions = append(b.Sessions, st)
		}
	}
	b.Disk = d.diskUse()
	if d.DB != nil {
		for _, q := range []struct {
			dst *int
			sql string
		}{
			{&b.Library.Movies, `SELECT COUNT(*) FROM movies`},
			{&b.Library.Series, `SELECT COUNT(*) FROM series WHERE kind = 'series'`},
			{&b.Library.Animes, `SELECT COUNT(*) FROM series WHERE kind = 'anime'`},
			{&b.Library.Files, `SELECT COUNT(*) FROM media_files`},
		} {
			if err := d.DB.QueryRowContext(ctx, q.sql).Scan(q.dst); err != nil {
				d.writeError(w, r, err)
				return
			}
		}
	}
	if d.Scanner != nil {
		b.Library.Review = d.Scanner.Status().Review
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, b)
}

func (d Deps) diskUse() diskUse {
	a := d.admin
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Since(a.at) < diskEvery {
		return a.disk
	}
	var u diskUse
	if d.Transcoder != nil {
		var dir string
		dir, u.TranscodeLimit = d.Transcoder.Cache()
		u.Transcode = dirSize(dir)
	}
	if d.DataDir != "" {
		u.Images = dirSize(filepath.Join(d.DataDir, "images"))
		u.Subtitles = dirSize(filepath.Join(d.DataDir, "subtitles"))
		for _, suffix := range []string{"", "-wal"} {
			if info, err := os.Stat(filepath.Join(d.DataDir, db.FileName+suffix)); err == nil {
				u.Database += info.Size()
			}
		}
	}
	a.disk, a.at = u, time.Now()
	return u
}

// dirSize adds up the files under dir (0 when it does not exist).
func dirSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, e fs.DirEntry, err error) error {
		if err == nil && e.Type().IsRegular() {
			if info, err := e.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil // a file removed while walking is fine
	})
	return n
}
