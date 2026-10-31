package api

import (
	"context"
	"crypto/rand"
	"encoding/json/v2"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"mediaserver/internal/stream"
	"mediaserver/internal/sysinfo"
)

// TestLoad: 3 streams at once, as in the Sprint 8 acceptance: 2 direct plays
// (whole file + seeks) and 1 conversion (when FFmpeg is in PATH).
// Run with `make load` (MEDIASERVER_LOAD=1); skipped in `make test`.
func TestLoad(t *testing.T) {
	if os.Getenv("MEDIASERVER_LOAD") == "" {
		t.Skip("load test: run with MEDIASERVER_LOAD=1 (make load)")
	}
	const directMB = 256
	direct := []string{"Filmes/Load A (2020)/Load A (2020).mp4", "Filmes/Load B (2021)/Load B (2021).mp4"}
	convert := "Filmes/Load C (2022)/Load C (2022).mkv"
	_, hasFFmpeg := exec.LookPath("ffmpeg")
	files := direct
	if hasFFmpeg == nil {
		files = append(slices.Clone(direct), convert)
	}
	srv, sc, fileID, conn := newLibraryServerDB(t, files...)
	root := sc.Root()

	// Direct play files: random bytes (the stream endpoint does not decode).
	for _, rel := range direct {
		f, err := os.Create(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.CopyN(f, rand.Reader, directMB<<20); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if hasFFmpeg == nil {
		// A real file the client "cannot play": MPEG-4 Part 2 + AC-3 in MKV.
		cmd := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-y",
			"-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30", "-f", "lavfi", "-i", "sine=frequency=440",
			"-t", "60", "-c:v", "mpeg4", "-q:v", "5", "-c:a", "ac3", filepath.Join(root, filepath.FromSlash(convert)))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("make test video: %v %s", err, out)
		}
	}
	if _, err := sc.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	probe := func(rel, container, video, audio string) {
		js := fmt.Sprintf(`{"streams": [
			{"index": 0, "codec_type": "video", "codec_name": %q, "pix_fmt": "yuv420p", "width": 1280, "height": 720},
			{"index": 1, "codec_type": "audio", "codec_name": %q, "channels": 2, "disposition": {"default": 1}}],
			"format": {"format_name": %q, "duration": "60"}}`, video, audio, container)
		if _, err := conn.ExecContext(t.Context(),
			`UPDATE media_files SET container = ?, video_codec = ?, duration_ms = 60000, probe_json = ? WHERE id = ?`,
			container, video, js, fileID(rel)); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range direct {
		probe(rel, "mp4", "h264", "aac")
	}
	if hasFFmpeg == nil {
		probe(convert, "mkv", "mpeg4", "ac3")
	}
	caps := `"caps": ["mp4", "h264", "aac"]`
	memBefore := sysinfo.Memory()

	var (
		mu     sync.Mutex
		ttfb   []time.Duration
		errs   []string
		bytesN int64
	)
	fail := func(format string, a ...any) {
		mu.Lock()
		errs = append(errs, fmt.Sprintf(format, a...))
		mu.Unlock()
	}
	client := srv.Client()
	get := func(ctx context.Context, url, rangeHdr string) (io.ReadCloser, time.Duration, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, 0, err
		}
		if rangeHdr != "" {
			req.Header.Set("Range", rangeHdr)
		}
		start := time.Now()
		res, err := client.Do(req)
		if err != nil {
			return nil, 0, err
		}
		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusPartialContent {
			res.Body.Close()
			return nil, 0, fmt.Errorf("GET %s: %d", url, res.StatusCode)
		}
		return res.Body, time.Since(start), nil
	}

	ctx := t.Context()
	// POST with the test's context (the goroutines below cannot call do).
	post := func(ctx context.Context, path, body string) (int, string) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+path, strings.NewReader(body))
		if err != nil {
			return 0, err.Error()
		}
		res, err := client.Do(req)
		if err != nil {
			return 0, err.Error()
		}
		defer res.Body.Close()
		data, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(data)
	}
	var wg sync.WaitGroup
	start := time.Now()
	for _, rel := range direct {
		wg.Go(func() {
			status, body := post(ctx, "/api/media/"+fileID(rel)+"/playback", "{"+caps+"}")
			var pb stream.Playback
			err := json.Unmarshal([]byte(body), &pb)
			if status != http.StatusOK || err != nil || !pb.Direct {
				fail("%s: playback %d %s", rel, status, body)
				return
			}
			// Three seeks, then the whole file from the start (a player
			// buffers ahead while it plays).
			for range 3 {
				off, _ := rand.Int(rand.Reader, big.NewInt(directMB<<20))
				b, d, err := get(ctx, srv.URL+pb.URL, fmt.Sprintf("bytes=%d-", off.Int64()))
				if err != nil {
					fail("%s seek: %v", rel, err)
					return
				}
				buf := make([]byte, 1<<20)
				_, err = io.ReadFull(b, buf)
				b.Close()
				if err != nil {
					fail("%s seek read: %v", rel, err)
				}
				mu.Lock()
				ttfb = append(ttfb, d)
				mu.Unlock()
			}
			b, d, err := get(ctx, srv.URL+pb.URL, "bytes=0-")
			if err != nil {
				fail("%s: %v", rel, err)
				return
			}
			n, err := io.Copy(io.Discard, b)
			b.Close()
			if err != nil || n != directMB<<20 {
				fail("%s: read %d bytes: %v", rel, n, err)
			}
			mu.Lock()
			ttfb = append(ttfb, d)
			bytesN += n
			mu.Unlock()
		})
	}

	var firstSegment time.Duration
	if hasFFmpeg == nil {
		wg.Go(func() {
			began := time.Now()
			status, body := post(ctx, "/api/media/"+fileID(convert)+"/playback", "{"+caps+"}")
			var pb stream.Playback
			err := json.Unmarshal([]byte(body), &pb)
			if status != http.StatusOK || err != nil || pb.Direct || pb.Video != "h264" {
				fail("convert: playback %d %s", status, body)
				return
			}
			base := srv.URL + strings.TrimSuffix(pb.URL, "index.m3u8")
			seen := map[string]bool{}
			for deadline := time.Now().Add(90 * time.Second); len(seen) < 5 && time.Now().Before(deadline); {
				b, _, err := get(ctx, srv.URL+pb.URL, "")
				if err != nil {
					time.Sleep(200 * time.Millisecond)
					continue
				}
				list, _ := io.ReadAll(b)
				b.Close()
				for line := range strings.SplitSeq(string(list), "\n") {
					if line == "" || strings.HasPrefix(line, "#") || seen[line] {
						continue
					}
					seg, _, err := get(ctx, base+line, "")
					if err != nil {
						fail("convert segment %s: %v", line, err)
						return
					}
					n, _ := io.Copy(io.Discard, seg)
					seg.Close()
					if len(seen) == 0 {
						firstSegment = time.Since(began)
					}
					seen[line] = true
					mu.Lock()
					bytesN += n
					mu.Unlock()
				}
				time.Sleep(200 * time.Millisecond)
			}
			if len(seen) < 5 {
				fail("convert: only %d segments in 90 s", len(seen))
			}
			req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, srv.URL+strings.TrimSuffix(pb.URL, "/index.m3u8"), nil)
			if res, err := client.Do(req); err == nil {
				res.Body.Close()
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)

	slices.Sort(ttfb)
	p95 := time.Duration(0)
	if len(ttfb) > 0 {
		p95 = ttfb[min(len(ttfb)-1, len(ttfb)*95/100)]
	}
	runtime.GC()
	memAfter := sysinfo.Memory()
	t.Logf("streams: %d direct + %d conversion in %v", len(direct), map[bool]int{true: 1}[hasFFmpeg == nil], elapsed.Round(time.Millisecond))
	t.Logf("direct play time to first byte: p95 %v, max %v (%d requests)", p95, ttfb[len(ttfb)-1], len(ttfb))
	t.Logf("read %.0f MB at %.0f MB/s", float64(bytesN)/(1<<20), float64(bytesN)/(1<<20)/elapsed.Seconds())
	if hasFFmpeg == nil {
		t.Logf("conversion: first segment after %v (CPU encoder in tests: HWACCEL=none)", firstSegment.Round(time.Millisecond))
	} else {
		t.Log("conversion skipped: no ffmpeg in PATH")
	}
	t.Logf("server memory: %.1f MB before, %.1f MB after", float64(memBefore.Bytes)/(1<<20), float64(memAfter.Bytes)/(1<<20))

	for _, e := range errs {
		t.Error(e)
	}
	if p95 > 300*time.Millisecond {
		t.Errorf("direct play p95 time to first byte %v > 300 ms (AGENTS.md §8)", p95)
	}
	// The files are streamed, never loaded: memory must not grow with them.
	if grow := int64(memAfter.Bytes) - int64(memBefore.Bytes); grow > 64<<20 {
		t.Errorf("memory grew %d MB while streaming", grow>>20)
	}
	if hasFFmpeg == nil {
		// Stopping the session ends FFmpeg: nothing left behind.
		call := func(method, path, body string, headers ...string) (int, string) {
			return do(t, srv, method, path, body, headers...)
		}
		_, header := newProfile(t, call, "Carga", "")
		for deadline := time.Now().Add(15 * time.Second); ; time.Sleep(250 * time.Millisecond) {
			_, body := call(http.MethodGet, "/api/admin/status", "", header)
			if strings.Contains(body, `"sessions":[]`) {
				break
			}
			if time.Now().After(deadline) {
				t.Errorf("FFmpeg still running 15 s after the session was stopped: %s", body)
				break
			}
		}
	}
}
