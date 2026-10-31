// Package stream serves media files to the clients: direct play (HTTP Range)
// and the generated images. Files are only ever opened through an os.Root,
// so nothing outside MEDIA_DIR can be served, even through symlinks.
package stream

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrNotFound is returned when a file does not exist or is outside the root.
var ErrNotFound = errors.New("not found")

// Resolver finds the relative path ('/' separators) of a playable media file.
type Resolver interface {
	PlayablePath(ctx context.Context, id string) (string, error)
}

// Server serves media files for direct play.
type Server struct {
	Root  *os.Root // MEDIA_DIR
	Files Resolver
}

var videoTypes = map[string]string{
	".mkv":  "video/x-matroska",
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".webm": "video/webm",
	".mov":  "video/quicktime",
	".avi":  "video/x-msvideo",
	".ts":   "video/mp2t",
}

// ServeMedia streams the media file id with Range support (http.ServeContent
// uses sendfile when it can). The file is never loaded in memory.
func (s *Server) ServeMedia(w http.ResponseWriter, r *http.Request, id string) error {
	rel, err := s.Files.PlayablePath(r.Context(), id)
	if err != nil {
		return err
	}
	f, info, err := openFile(s.Root, rel)
	if err != nil {
		return err
	}
	defer f.Close()

	ext := strings.ToLower(path.Ext(rel))
	ctype := videoTypes[ext]
	if ctype == "" {
		ctype = mime.TypeByExtension(ext)
	}
	if ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	// The file changes only together with its mtime and size.
	w.Header().Set("ETag", fmt.Sprintf(`"%x-%x"`, info.ModTime().UnixNano(), info.Size()))
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "", info.ModTime(), f)
	return nil
}

// openFile opens rel inside root; anything escaping it or not a regular file
// is reported as ErrNotFound.
func openFile(root *os.Root, rel string) (*os.File, fs.FileInfo, error) {
	f, err := root.Open(filepath.FromSlash(rel))
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w: %w", rel, ErrNotFound, err)
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, fmt.Errorf("open %s: %w: not a regular file", rel, ErrNotFound)
	}
	return f, info, nil
}
