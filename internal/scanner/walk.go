package scanner

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
)

// fileEntry is a file found on disk (only metadata, never content).
type fileEntry struct {
	rel   string // relative to the media root, "/" separators
	size  int64
	mtime int64 // Unix nanoseconds
}

type walkResult struct {
	videos   []fileEntry
	sidecars []string // subtitles and images, relative paths
	unstable map[string]bool
}

// skipDirs are system/trash folders never worth scanning.
var skipDirs = setOf("@eadir", "$recycle.bin", "system volume information", "lost+found", "#recycle", ".trash")

// walk lists video files and sidecars under root. Files modified less than
// minAge ago are reported as unstable (probably still being copied).
// Symlinks are not followed, so nothing outside root is ever read.
func walk(root string, minAge time.Duration, now time.Time, log *slog.Logger) (walkResult, error) {
	res := walkResult{unstable: map[string]bool{}}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			log.Warn("scan: skipping unreadable path", "path", path, "err", err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if path != root && (strings.HasPrefix(name, ".") || skipDirs[strings.ToLower(name)]) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() {
			return nil
		}
		video, side := IsVideo(name), IsSubtitle(name) || IsImage(name)
		if !video && !side {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("rel %s: %w", path, err)
		}
		rel = filepath.ToSlash(rel)
		if side {
			res.sidecars = append(res.sidecars, rel)
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // removed during the walk
			}
			log.Warn("scan: stat failed", "path", path, "err", err)
			return nil
		}
		if minAge > 0 && now.Sub(info.ModTime()) < minAge {
			res.unstable[rel] = true
			return nil
		}
		res.videos = append(res.videos, fileEntry{rel: rel, size: info.Size(), mtime: info.ModTime().UnixNano()})
		return nil
	})
	if err != nil {
		return res, fmt.Errorf("walk %s: %w", root, err)
	}
	return res, nil
}
