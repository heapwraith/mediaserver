package stream

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type fakeFiles map[string]string

func (f fakeFiles) PlayablePath(_ context.Context, id string) (string, error) {
	if rel, ok := f[id]; ok {
		return rel, nil
	}
	return "", ErrNotFound
}

func newServer(t testing.TB, files fakeFiles) (*Server, string) {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, "média ç 日本")
	if err := os.MkdirAll(filepath.Join(dir, "Sub Folder"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "secret.mkv"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return &Server{Root: root, Files: files}, dir
}

func TestServeMedia(t *testing.T) {
	files := fakeFiles{
		"ok":       "Sub Folder/Filme é (2020).mp4",
		"escape":   "../secret.mkv",
		"absolute": "",
		"dir":      "Sub Folder",
		"link":     "link.mkv",
	}
	s, dir := newServer(t, files)
	files["absolute"] = filepath.ToSlash(filepath.Join(filepath.Dir(dir), "secret.mkv"))
	if err := os.WriteFile(filepath.Join(dir, "Sub Folder", "Filme é (2020).mp4"), []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkOK := os.Symlink(filepath.Join(filepath.Dir(dir), "secret.mkv"), filepath.Join(dir, "link.mkv")) == nil

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	req.Header.Set("Range", "bytes=2-4")
	if err := s.ServeMedia(rec, req, "ok"); err != nil || rec.Code != 206 || rec.Body.String() != "234" ||
		rec.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("range: %v %d %q %v", err, rec.Code, rec.Body.String(), rec.Header())
	}

	for _, id := range []string{"escape", "absolute", "dir", "link", "unknown"} {
		if id == "link" && !symlinkOK {
			continue // creating symlinks needs privileges on Windows
		}
		rec := httptest.NewRecorder()
		err := s.ServeMedia(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil), id)
		if !errors.Is(err, ErrNotFound) || rec.Body.Len() != 0 {
			t.Errorf("%s: err %v, body %q", id, err, rec.Body.String())
		}
	}
}

func BenchmarkServeRange(b *testing.B) {
	s, dir := newServer(b, fakeFiles{"v": "v.mkv"})
	data := make([]byte, 64<<20)
	if err := os.WriteFile(filepath.Join(dir, "v.mkv"), data, 0o600); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(1 << 20)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		start := (i % 60) << 20
		req := httptest.NewRequestWithContext(b.Context(), http.MethodGet, "/", nil)
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, start+1<<20-1))
		rec := httptest.NewRecorder()
		if err := s.ServeMedia(rec, req, "v"); err != nil || rec.Code != 206 {
			b.Fatal(err, rec.Code)
		}
	}
}

func TestPickFrame(t *testing.T) {
	solid := func(v uint8) image.Image {
		img := image.NewGray(image.Rect(0, 0, 64, 36))
		for i := range img.Pix {
			img.Pix[i] = v
		}
		return img
	}
	pattern := func(step int, hi, lo uint8) image.Image { // checkerboard: smaller step = more detail
		img := image.NewGray(image.Rect(0, 0, 64, 36))
		for y := range 36 {
			for x := range 64 {
				if (x/step+y/step)%2 == 0 {
					img.SetGray(x, y, color.Gray{Y: hi})
				} else {
					img.SetGray(x, y, color.Gray{Y: lo})
				}
			}
		}
		return img
	}
	tests := []struct {
		name   string
		frames []image.Image
		want   int
	}{
		{"none", nil, -1},
		{"black is skipped", []image.Image{solid(2), pattern(8, 200, 40)}, 1},
		{"flat gray is skipped", []image.Image{solid(128), pattern(8, 200, 40)}, 1},
		{"sharpest wins", []image.Image{pattern(8, 200, 40), pattern(2, 200, 40), pattern(16, 200, 40)}, 1},
		{"all dark: most contrasted", []image.Image{solid(1), pattern(4, 12, 2), solid(3)}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stats []frameStats
			for _, f := range tt.frames {
				stats = append(stats, analyze(f))
			}
			if got := pickFrame(stats); got != tt.want {
				t.Fatalf("pickFrame = %d, want %d (%+v)", got, tt.want, stats)
			}
		})
	}
}
