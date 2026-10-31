package scanner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatchOptions tune the watcher debounce.
type WatchOptions struct {
	// Quiet is how long the folder must stay silent before a scan starts.
	Quiet time.Duration
	// Settle is the interval of the final size/mtime check: a file that still
	// grows (a copy in progress) postpones the scan.
	Settle time.Duration
}

func (o *WatchOptions) defaults() {
	if o.Quiet <= 0 {
		o.Quiet = 2 * time.Second
	}
	if o.Settle <= 0 {
		o.Settle = time.Second
	}
}

// Watch watches the media folder (recursively) and runs an incremental scan
// after changes settle. It blocks until ctx is canceled.
func (s *Scanner) Watch(ctx context.Context, opt WatchOptions) error {
	opt.defaults()
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("watcher: %w", err)
	}
	defer w.Close()
	if err := s.addTree(w, s.opt.Root); err != nil {
		return err
	}
	s.log.Info("watching media folder", "root", s.opt.Root)

	pending := map[string]bool{} // changed paths since the last scan
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	arm := func() { timer.Reset(opt.Quiet) }

	for {
		select {
		case <-ctx.Done():
			return nil

		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Op == fsnotify.Chmod || s.ignored(ev.Name) {
				continue
			}
			if ev.Has(fsnotify.Create) {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					// a new folder (maybe with files already inside)
					if err := s.addTree(w, ev.Name); err != nil {
						s.log.Warn("watcher: cannot watch new folder", "path", ev.Name, "err", err)
					}
				}
			}
			pending[ev.Name] = true
			arm()

		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			s.log.Warn("watcher error", "err", err)
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				pending[s.opt.Root] = true // too many events: just rescan
				arm()
			}

		case <-timer.C:
			if len(pending) == 0 {
				continue
			}
			if changing := stillChanging(ctx, pending, opt.Settle); changing {
				arm() // a copy is still running: wait more
				continue
			}
			clear(pending)
			s.Start(ctx)
		}
	}
}

// addTree watches dir and all its subfolders (fsnotify is not recursive).
func (s *Scanner) addTree(w *fsnotify.Watcher, dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return fmt.Errorf("watch %s: %w", path, err)
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != s.opt.Root && s.ignored(path) {
			return filepath.SkipDir
		}
		if err := w.Add(path); err != nil {
			s.log.Warn("watcher: cannot watch folder", "path", path, "err", err)
		}
		return nil
	})
}

// ignored reports hidden/system paths (same rules as the scan).
func (s *Scanner) ignored(path string) bool {
	rel, err := filepath.Rel(s.opt.Root, path)
	if err != nil || rel == "." {
		return false
	}
	for part := range strings.SplitSeq(filepath.ToSlash(rel), "/") {
		if strings.HasPrefix(part, ".") || skipDirs[strings.ToLower(part)] {
			return true
		}
	}
	return false
}

// stillChanging compares size and mtime of the pending files before and
// after settle. Folders and removed files count as settled.
func stillChanging(ctx context.Context, pending map[string]bool, settle time.Duration) bool {
	type stamp struct {
		size  int64
		mtime time.Time
	}
	read := func() map[string]stamp {
		out := make(map[string]stamp, len(pending))
		for p := range pending {
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				out[p] = stamp{info.Size(), info.ModTime()}
			}
		}
		return out
	}
	before := read()
	if len(before) == 0 {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(settle):
	}
	after := read()
	for p, b := range before {
		if a, ok := after[p]; ok && a != b {
			return true
		}
	}
	return false
}
