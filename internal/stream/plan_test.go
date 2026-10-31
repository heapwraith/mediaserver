package stream

import (
	"testing"

	"mediaserver/internal/library"
	"mediaserver/internal/probe"
)

func TestDecide(t *testing.T) {
	chrome := []string{"mp4", "webm", "mkv", "h264", "hevc", "hevc:10", "av1", "vp9", "aac", "mp3", "opus", "flac"}
	firefox := []string{"mp4", "webm", "h264", "av1", "vp9", "aac", "mp3", "opus", "flac"}
	safari := []string{"mp4", "mov", "h264", "hevc", "hevc:10", "aac", "mp3", "ac3", "eac3", "flac"}

	// The test file of media/: MKV H.264 + AAC (pt, default) + E-AC-3 5.1 (en).
	dual := library.Media{Container: "mkv", VideoCodec: "h264", VideoBits: 8, Audio: []probe.Track{
		{Index: 1, Codec: "aac", Default: true}, {Index: 2, Codec: "eac3", Channels: 6},
	}}
	// MKV HEVC 10-bit + DTS (the Sprint 5 acceptance file).
	hevcDTS := library.Media{Container: "mkv", VideoCodec: "hevc", VideoBits: 10, Audio: []probe.Track{
		{Index: 1, Codec: "dts", Channels: 6},
	}}
	mp4 := library.Media{Container: "mp4", VideoCodec: "h264", Audio: []probe.Track{{Index: 1, Codec: "aac"}}}
	xvid := library.Media{Container: "avi", VideoCodec: "mpeg4", Audio: []probe.Track{{Index: 1, Codec: "mp3"}}}
	silent := library.Media{Container: "mkv", VideoCodec: "h264"}

	tests := []struct {
		name  string
		m     library.Media
		caps  []string
		audio int
		want  Plan
	}{
		{"mp4 everywhere", mp4, firefox, 0, Plan{Direct: true, AudioIndex: -1}},
		{"mkv default track in chrome", dual, chrome, 1, Plan{Direct: true, AudioIndex: -1}},
		{"no audio index = default track", dual, chrome, -1, Plan{Direct: true, AudioIndex: -1}},
		{"second track: remux + aac", dual, chrome, 2, Plan{Video: "copy", VideoCodec: "h264", Audio: "aac", AudioIndex: 2}},
		{"second track in safari: copy eac3", dual, safari, 2, Plan{Video: "copy", VideoCodec: "h264", Audio: "copy", AudioIndex: 2}},
		{"mkv in firefox: remux", dual, firefox, 1, Plan{Video: "copy", VideoCodec: "h264", Audio: "copy", AudioIndex: 1}},
		{"hevc 10-bit + dts in chrome", hevcDTS, chrome, 1, Plan{Video: "copy", VideoCodec: "hevc", Audio: "aac", AudioIndex: 1}},
		{"hevc 10-bit + dts in firefox", hevcDTS, firefox, 1, Plan{Video: "h264", VideoCodec: "hevc", Audio: "aac", AudioIndex: 1}},
		{"hevc 10-bit + dts in safari", hevcDTS, safari, 1, Plan{Video: "copy", VideoCodec: "hevc", Audio: "aac", AudioIndex: 1}},
		{"8-bit only client", hevcDTS, []string{"mkv", "hevc", "aac"}, 1, Plan{Video: "h264", VideoCodec: "hevc", Audio: "aac", AudioIndex: 1}},
		{"xvid: transcode, mp3 copied", xvid, chrome, 1, Plan{Video: "h264", VideoCodec: "mpeg4", Audio: "copy", AudioIndex: 1}},
		{"no caps", mp4, nil, 1, Plan{Video: "h264", VideoCodec: "h264", Audio: "aac", AudioIndex: 1}},
		{"no audio", silent, firefox, 0, Plan{Video: "copy", VideoCodec: "h264", AudioIndex: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decide(tt.m, tt.caps, tt.audio); got != tt.want {
				t.Errorf("Decide = %+v, want %+v", got, tt.want)
			}
		})
	}
}
