package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitFor polls cond until it is true or the timeout expires.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return cond()
}

func TestWatcherIndexesCopiedEpisode(t *testing.T) {
	e := newEnv(t)
	e.sc.opt.MinAge = 300 * time.Millisecond
	e.write(t, "Series/Dark/Season 1/Dark.S01E01.mkv")
	e.scan(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- e.sc.Watch(ctx, WatchOptions{Quiet: 300 * time.Millisecond, Settle: 200 * time.Millisecond})
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	time.Sleep(200 * time.Millisecond) // let the watcher register the folders

	episodes := func() int { return e.count(t, `SELECT count(*) FROM episodes`) }

	// Copy a new episode slowly, in chunks: nothing may be indexed mid-copy.
	rel := "Series/Dark/Season 1/Dark.S01E02.mkv"
	p := filepath.Join(e.root, filepath.FromSlash(rel))
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	chunk := make([]byte, 64<<10)
	for i := range 8 {
		chunk[0] = byte(i)
		if _, err := f.Write(chunk); err != nil {
			t.Fatal(err)
		}
		time.Sleep(150 * time.Millisecond)
		if i == 5 && e.count(t, `SELECT count(*) FROM media_files WHERE rel_path = ?`, rel) != 0 {
			t.Fatal("file indexed while it was still being copied")
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, 10*time.Second, func() bool { return episodes() == 2 }) {
		t.Fatalf("copied episode not indexed (episodes=%d, status=%+v)", episodes(), e.sc.Status())
	}
	// Indexed with its final size (the whole copy), not a partial one.
	if n := e.count(t, `SELECT size FROM media_files WHERE rel_path = ?`, rel); n != 8*64<<10 {
		t.Fatalf("indexed size %d, want %d", n, 8*64<<10)
	}

	// A folder created after the watcher started is watched too.
	e.write(t, "Series/New Show/Season 1/New.Show.S01E01.mkv")
	if !waitFor(t, 10*time.Second, func() bool { return episodes() == 3 }) {
		t.Fatalf("episode in a new folder not indexed (episodes=%d)", episodes())
	}

	// Deleting a file removes it without a manual scan.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, 10*time.Second, func() bool { return episodes() == 2 }) {
		t.Fatalf("deleted episode still in the library (episodes=%d)", episodes())
	}
}
