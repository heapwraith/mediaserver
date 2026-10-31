package probe

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const mkvJSON = `{
  "streams": [
    {"index": 0, "codec_type": "video", "codec_name": "hevc", "width": 3840, "height": 2160,
     "pix_fmt": "yuv420p10le", "color_transfer": "smpte2084", "disposition": {"default": 1}},
    {"index": 1, "codec_type": "audio", "codec_name": "dts", "channels": 6,
     "disposition": {"default": 0}, "tags": {"language": "eng", "title": "DTS-HD MA"}},
    {"index": 2, "codec_type": "audio", "codec_name": "aac", "channels": 2,
     "disposition": {"default": 1}, "tags": {"language": "jpn"}},
    {"index": 3, "codec_type": "subtitle", "codec_name": "ass",
     "disposition": {"default": 1, "forced": 0}, "tags": {"language": "por", "title": "Português"}},
    {"index": 4, "codec_type": "subtitle", "codec_name": "subrip",
     "disposition": {"default": 0, "forced": 1}, "tags": {"language": "pob"}},
    {"index": 5, "codec_type": "video", "codec_name": "mjpeg", "disposition": {"attached_pic": 1}}
  ],
  "format": {"format_name": "matroska,webm", "duration": "1425.300000", "bit_rate": "12000000"}
}`

func TestParse(t *testing.T) {
	res, err := Parse([]byte(mkvJSON), ".mkv")
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name      string
		got, want any
	}{
		{"container", res.Container, "mkv"},
		{"duration", res.DurationMs, int64(1425300)},
		{"bitrate", res.Bitrate, int64(12000000)},
		{"video codec", res.VideoCodec, "hevc"},
		{"resolution", [2]int{res.Width, res.Height}, [2]int{3840, 2160}},
		{"bit depth", res.VideoBits, 10},
		{"hdr", res.HDR, "hdr10"},
		{"default audio codec", res.AudioCodec, "aac"},
		{"audio tracks", len(res.Audio), 2},
		{"audio 0 lang", res.Audio[0].Language, "en"},
		{"audio 0 channels", res.Audio[0].Channels, 6},
		{"audio 1 lang", res.Audio[1].Language, "ja"},
		{"subtitle tracks", len(res.Subtitles), 2},
		{"sub 0", res.Subtitles[0], Track{Index: 3, Codec: "ass", Language: "pt", Title: "Português", Default: true}},
		{"sub 1", res.Subtitles[1], Track{Index: 4, Codec: "subrip", Language: "pt-BR", Forced: true}},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestParseVariants(t *testing.T) {
	tests := []struct {
		name, json, ext string
		container, hdr  string
		wantErr         error
	}{
		{
			"mp4 family", `{"streams":[{"codec_type":"video","codec_name":"h264"}],"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2"}}`,
			".mp4", "mp4", "", nil,
		},
		{
			"m4v keeps ext", `{"streams":[{"codec_type":"video","codec_name":"h264"}],"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2"}}`,
			".m4v", "m4v", "", nil,
		},
		{
			"webm", `{"streams":[{"codec_type":"video","codec_name":"vp9"}],"format":{"format_name":"matroska,webm"}}`,
			".webm", "webm", "", nil,
		},
		{
			"ts + hlg", `{"streams":[{"codec_type":"video","codec_name":"hevc","color_transfer":"arib-std-b67"}],"format":{"format_name":"mpegts"}}`,
			".ts", "ts", "hlg", nil,
		},
		{
			"dolby vision", `{"streams":[{"codec_type":"video","codec_name":"hevc","color_transfer":"smpte2084","side_data_list":[{"side_data_type":"DOVI configuration record"}]}],"format":{"format_name":"matroska,webm"}}`,
			".mkv", "mkv", "dolby_vision", nil,
		},
		{"no streams", `{"streams":[],"format":{"format_name":"matroska,webm"}}`, ".mkv", "", "", ErrNoStreams},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Parse([]byte(tt.json), tt.ext)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if res.Container != tt.container || res.HDR != tt.hdr {
				t.Errorf("container=%q hdr=%q, want %q %q", res.Container, res.HDR, tt.container, tt.hdr)
			}
		})
	}
	if _, err := Parse([]byte("not json"), ".mkv"); err == nil {
		t.Error("invalid json: want error")
	}
}

// ffprobePath returns FFPROBE_PATH or ffprobe from PATH; the test is skipped
// when neither exists.
func ffprobePath(t *testing.T) (ffprobe, ffmpeg string) {
	t.Helper()
	ffprobe, ffmpeg = os.Getenv("FFPROBE_PATH"), os.Getenv("FFMPEG_PATH")
	if ffprobe == "" {
		ffprobe, _ = exec.LookPath("ffprobe")
	}
	if ffmpeg == "" {
		ffmpeg, _ = exec.LookPath("ffmpeg")
	}
	if ffprobe == "" || ffmpeg == "" {
		t.Skip("ffprobe/ffmpeg not found (set FFPROBE_PATH and FFMPEG_PATH)")
	}
	return ffprobe, ffmpeg
}

// TestProbeRealFile generates a 2 s H.264 + AAC + SRT MKV and probes it.
func TestProbeRealFile(t *testing.T) {
	ffprobe, ffmpeg := ffprobePath(t)
	dir := t.TempDir()
	srt := filepath.Join(dir, "sub.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:01,000\nOlá\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "Série com acento ação.mkv")
	//nolint:gosec // test-only command with fixed arguments
	gen := exec.CommandContext(t.Context(), ffmpeg, "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=2",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-i", srt,
		"-map", "0", "-map", "1", "-map", "2",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-c:s", "srt",
		"-metadata:s:a:0", "language=jpn", "-metadata:s:s:0", "language=por",
		file)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}

	p := Prober{Path: ffprobe}
	if err := p.Available(t.Context()); err != nil {
		t.Fatal(err)
	}
	res, err := p.Probe(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	if res.Container != "mkv" || res.VideoCodec != "h264" || res.AudioCodec != "aac" {
		t.Errorf("got container=%s video=%s audio=%s", res.Container, res.VideoCodec, res.AudioCodec)
	}
	if res.Width != 320 || res.Height != 240 {
		t.Errorf("resolution %dx%d, want 320x240", res.Width, res.Height)
	}
	if res.DurationMs < 1900 || res.DurationMs > 2200 {
		t.Errorf("duration %d ms, want ~2000", res.DurationMs)
	}
	if len(res.Audio) != 1 || res.Audio[0].Language != "ja" {
		t.Errorf("audio tracks = %+v", res.Audio)
	}
	if len(res.Subtitles) != 1 || res.Subtitles[0].Language != "pt" || res.Subtitles[0].Codec != "subrip" {
		t.Errorf("subtitle tracks = %+v", res.Subtitles)
	}

	// Not a media file: clear error, no panic.
	bad := filepath.Join(dir, "fake.mkv")
	if err := os.WriteFile(bad, []byte("not a video"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Probe(t.Context(), bad); err == nil || !strings.Contains(err.Error(), "probe") {
		t.Errorf("probe of fake file: err = %v", err)
	}
}
