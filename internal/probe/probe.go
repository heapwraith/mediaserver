// Package probe reads technical metadata of media files with ffprobe.
package probe

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultTimeout bounds a single ffprobe run.
const DefaultTimeout = 30 * time.Second

// Track is an audio or subtitle stream.
type Track struct {
	Index    int    `json:"index"`
	Codec    string `json:"codec"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Channels int    `json:"channels,omitempty"`
	Default  bool   `json:"default,omitempty"`
	Forced   bool   `json:"forced,omitempty"`
}

// Result is what the server needs from ffprobe.
type Result struct {
	Container  string  `json:"container"`
	DurationMs int64   `json:"duration_ms"`
	Bitrate    int64   `json:"bitrate"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	VideoCodec string  `json:"video_codec"`
	VideoBits  int     `json:"video_bits,omitzero"` // bit depth of the video (8, 10, 12)
	HDR        string  `json:"hdr,omitempty"`       // hdr10, hlg, dolby_vision
	FPS        float64 `json:"fps,omitzero"`        // frames per second of the video
	AudioCodec string  `json:"audio_codec"`         // default (or first) audio track
	Audio      []Track `json:"audio"`
	Subtitles  []Track `json:"subtitles"`
	// Raw is the full ffprobe JSON (kept for tracks/chapters in later sprints).
	Raw []byte `json:"-"`
}

// Prober runs ffprobe.
type Prober struct {
	Path    string        // ffprobe binary
	Timeout time.Duration // per file; DefaultTimeout when zero
}

// Available checks that the ffprobe binary runs.
func (p Prober) Available(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	//nolint:gosec // the binary path comes from the operator's config (FFPROBE_PATH)
	if err := exec.CommandContext(ctx, p.Path, "-version").Run(); err != nil {
		return fmt.Errorf("ffprobe %s: %w", p.Path, err)
	}
	return nil
}

// Probe reads the streams and format of file. The process is killed when ctx
// is canceled (AGENTS.md §5.5).
func (p Prober) Probe(ctx context.Context, file string) (*Result, error) {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	//nolint:gosec // fixed arguments; the binary path comes from the operator's config
	cmd := exec.CommandContext(ctx, p.Path,
		"-v", "error", "-print_format", "json", "-show_format", "-show_streams", "-i", file)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, fmt.Errorf("probe %s: %w: %s", file, err, msg)
	}
	res, err := Parse(out, filepath.Ext(file))
	if err != nil {
		return nil, fmt.Errorf("probe %s: %w", file, err)
	}
	return res, nil
}

type ffStream struct {
	Index         int    `json:"index"`
	CodecType     string `json:"codec_type"`
	CodecName     string `json:"codec_name"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	Channels      int    `json:"channels"`
	PixFmt        string `json:"pix_fmt"`
	ColorTransfer string `json:"color_transfer"`
	AvgFrameRate  string `json:"avg_frame_rate"` // "24000/1001"
	RFrameRate    string `json:"r_frame_rate"`
	SideDataList  []struct {
		SideDataType string `json:"side_data_type"`
	} `json:"side_data_list"`
	Disposition struct {
		Default        int `json:"default"`
		Forced         int `json:"forced"`
		AttachedPic    int `json:"attached_pic"`
		HearingImpared int `json:"hearing_impaired"`
	} `json:"disposition"`
	Tags map[string]string `json:"tags"`
}

type ffOutput struct {
	Streams []ffStream `json:"streams"`
	Format  struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
}

// ErrNoStreams is returned when ffprobe finds no audio or video stream.
var ErrNoStreams = errors.New("no audio or video stream")

// Parse converts ffprobe JSON output. ext (".mkv") disambiguates the
// container when ffprobe reports a family ("mov,mp4,m4a,...").
func Parse(data []byte, ext string) (*Result, error) {
	var ff ffOutput
	if err := json.Unmarshal(data, &ff); err != nil {
		return nil, fmt.Errorf("decode ffprobe json: %w", err)
	}
	res := &Result{
		Container: container(ff.Format.FormatName, ext),
		Raw:       data,
	}
	if d, err := strconv.ParseFloat(ff.Format.Duration, 64); err == nil {
		res.DurationMs = int64(d * 1000)
	}
	if b, err := strconv.ParseInt(ff.Format.BitRate, 10, 64); err == nil {
		res.Bitrate = b
	}

	defaultAudio := -1
	for _, s := range ff.Streams {
		switch s.CodecType {
		case "video":
			if s.Disposition.AttachedPic == 1 || res.VideoCodec != "" {
				continue // cover art, or a second video stream
			}
			res.VideoCodec = s.CodecName
			res.Width, res.Height = s.Width, s.Height
			res.VideoBits = bitDepth(s.PixFmt)
			res.HDR = hdr(s)
			res.FPS = frameRate(s)
		case "audio":
			t := track(s)
			t.Channels = s.Channels
			if t.Default && defaultAudio < 0 {
				defaultAudio = len(res.Audio)
			}
			res.Audio = append(res.Audio, t)
		case "subtitle":
			res.Subtitles = append(res.Subtitles, track(s))
		}
	}
	if len(res.Audio) > 0 {
		res.AudioCodec = res.Audio[max(defaultAudio, 0)].Codec
	}
	if res.VideoCodec == "" && len(res.Audio) == 0 {
		return nil, ErrNoStreams
	}
	return res, nil
}

func track(s ffStream) Track {
	return Track{
		Index:    s.Index,
		Codec:    s.CodecName,
		Language: language(s.Tags["language"]),
		Title:    s.Tags["title"],
		Default:  s.Disposition.Default == 1,
		Forced:   s.Disposition.Forced == 1,
	}
}

// frameRate reads the frames per second; ffprobe answers "0/0" when it does not know.
func frameRate(s ffStream) float64 {
	for _, r := range []string{s.AvgFrameRate, s.RFrameRate} {
		num, den, _ := strings.Cut(r, "/")
		n, errN := strconv.ParseFloat(num, 64)
		d, errD := strconv.ParseFloat(den, 64)
		if errN == nil && errD == nil && n > 0 && d > 0 {
			return n / d
		}
	}
	return 0
}

// bitDepth reads the bit depth from a pixel format (yuv420p10le, p010le…).
func bitDepth(pixFmt string) int {
	switch {
	case pixFmt == "":
		return 0
	case strings.Contains(pixFmt, "12"):
		return 12
	case strings.Contains(pixFmt, "10"):
		return 10
	}
	return 8
}

func hdr(s ffStream) string {
	for _, sd := range s.SideDataList {
		if strings.Contains(strings.ToLower(sd.SideDataType), "dovi") ||
			strings.Contains(strings.ToLower(sd.SideDataType), "dolby vision") {
			return "dolby_vision"
		}
	}
	switch s.ColorTransfer {
	case "smpte2084":
		return "hdr10"
	case "arib-std-b67":
		return "hlg"
	}
	return ""
}

func container(formatName, ext string) string {
	ext = strings.TrimPrefix(strings.ToLower(ext), ".")
	switch {
	case strings.Contains(formatName, "matroska"):
		if ext == "webm" {
			return "webm"
		}
		return "mkv"
	case strings.Contains(formatName, "mp4"), strings.Contains(formatName, "mov"):
		switch ext {
		case "mov", "m4v":
			return ext
		}
		return "mp4"
	case formatName == "mpegts":
		return "ts"
	case formatName == "":
		return ext
	}
	if i := strings.IndexByte(formatName, ','); i > 0 {
		return formatName[:i]
	}
	return formatName
}

// language maps ffprobe's ISO 639-2 tags to short BCP 47 codes.
func language(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if l, ok := iso6392[tag]; ok {
		return l
	}
	if tag == "und" {
		return ""
	}
	return tag
}

var iso6392 = map[string]string{
	"eng": "en", "por": "pt", "pob": "pt-BR", "pt-br": "pt-BR", "jpn": "ja", "spa": "es",
	"fre": "fr", "fra": "fr", "ger": "de", "deu": "de", "ita": "it", "kor": "ko",
	"chi": "zh", "zho": "zh", "rus": "ru", "ara": "ar", "hin": "hi", "pol": "pl",
	"dut": "nl", "nld": "nl", "swe": "sv", "nor": "no", "dan": "da", "fin": "fi",
	"tur": "tr", "tha": "th", "vie": "vi", "ind": "id", "may": "ms", "msa": "ms",
	"heb": "he", "gre": "el", "ell": "el", "cze": "cs", "ces": "cs", "hun": "hu",
	"rum": "ro", "ron": "ro", "ukr": "uk", "lat": "la",
}
