package stream

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"
)

// encoder is the H.264 encoder of the video transcodes. Frames are decoded
// (on the GPU when -hwaccel auto finds one) and scaled in system memory,
// then each encoder uploads them itself; VAAPI needs the explicit upload.
type encoder struct {
	Name   string
	input  []string // options before -i
	format string   // last filter of the chain: pixel format (+ upload)
	args   []string
}

var encoders = map[string]encoder{
	"nvenc": {
		Name: "nvenc", input: []string{"-hwaccel", "auto"}, format: "format=yuv420p",
		args: []string{
			"-c:v", "h264_nvenc", "-preset", "p4", "-rc", "vbr", "-cq", "23", "-b:v", "0",
			"-maxrate", "15M", "-bufsize", "30M", "-forced-idr", "1",
		},
	},
	"qsv": {
		Name: "qsv", input: []string{"-hwaccel", "auto"}, format: "format=nv12",
		args: []string{"-c:v", "h264_qsv", "-preset", "veryfast", "-global_quality", "23", "-forced_idr", "1"},
	},
	"amf": {
		Name: "amf", input: []string{"-hwaccel", "auto"}, format: "format=yuv420p",
		args: []string{
			"-c:v", "h264_amf", "-quality", "speed", "-rc", "vbr_peak", "-b:v", "8M",
			"-maxrate", "15M", "-forced_idr", "1",
		},
	},
	"vaapi": {
		Name: "vaapi", input: []string{"-vaapi_device", "/dev/dri/renderD128"}, format: "format=nv12,hwupload",
		args: []string{"-c:v", "h264_vaapi", "-rc_mode", "VBR", "-b:v", "8M", "-maxrate", "15M"},
	},
	"videotoolbox": {
		Name: "videotoolbox", input: []string{"-hwaccel", "auto"}, format: "format=yuv420p",
		args: []string{"-c:v", "h264_videotoolbox", "-b:v", "8M", "-maxrate", "15M"},
	},
	"cpu": {
		Name: "cpu", format: "format=yuv420p",
		args: []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "21", "-maxrate", "15M", "-bufsize", "30M"},
	},
}

// autoOrder is the order HWACCEL=auto tries the GPUs of each system.
var autoOrder = map[string][]string{
	"windows": {"nvenc", "qsv", "amf"},
	"linux":   {"nvenc", "vaapi", "qsv"},
	"darwin":  {"videotoolbox"},
}

// detectEncoder picks the encoder for mode (config HWACCEL): the first GPU
// encoder that really encodes a short test clip, else libx264.
func detectEncoder(ctx context.Context, ffmpeg, mode string, log *slog.Logger) encoder {
	var names []string
	switch mode {
	case "none":
	case "auto":
		names = autoOrder[runtime.GOOS]
	default:
		names = []string{mode}
	}
	for _, name := range names {
		enc, ok := encoders[name]
		if !ok {
			continue
		}
		err := testEncoder(ctx, ffmpeg, enc)
		if err == nil {
			log.Info("video encoder", "encoder", name)
			return enc
		}
		if mode != "auto" {
			log.Warn("HWACCEL encoder unavailable, using the CPU", "hwaccel", mode, "err", err)
		}
	}
	log.Info("video encoder", "encoder", "cpu")
	return encoders["cpu"]
}

// testEncoder encodes 0.2 s of a generated clip with the encoder's options.
func testEncoder(ctx context.Context, ffmpeg string, enc encoder) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	args := slices.Concat([]string{"-hide_banner", "-nostdin", "-loglevel", "error"},
		withoutHWAccel(enc.input),
		[]string{"-f", "lavfi", "-i", "color=c=black:s=256x144:r=24:d=0.2", "-vf", enc.format},
		enc.args, []string{"-f", "null", "-"})
	//nolint:gosec // the binary comes from the operator's config (FFMPEG_PATH); fixed arguments
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(firstLine(stderr.String())))
	}
	return nil
}

// withoutHWAccel drops "-hwaccel x": a generated clip has nothing to decode.
func withoutHWAccel(in []string) []string {
	if i := slices.Index(in, "-hwaccel"); i >= 0 && i+1 < len(in) {
		return slices.Delete(slices.Clone(in), i, i+2)
	}
	return in
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
