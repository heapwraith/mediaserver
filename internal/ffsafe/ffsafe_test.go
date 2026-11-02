// SPDX-License-Identifier: GPL-3.0-or-later

package ffsafe

import (
	"slices"
	"strings"
	"testing"
)

func TestOptions(t *testing.T) {
	for _, net := range []string{"http", "https", "tcp", "udp", "rtmp", "ftp", "data", "crypto", "concat", "subfile"} {
		if slices.Contains(strings.Split(Protocols, ","), net) {
			t.Errorf("protocol %q must not be allowed", net)
		}
	}
	for _, f := range []string{"hls", "dash", "concat", "image2", "lavfi", "sdp", "rtsp"} {
		if slices.Contains(strings.Split(VideoFormats, ","), f) {
			t.Errorf("demuxer %q must not be allowed for video", f)
		}
	}
	for _, f := range []string{"matroska", "webm", "mov", "mp4", "avi", "mpegts"} {
		if !slices.Contains(strings.Split(VideoFormats, ","), f) {
			t.Errorf("demuxer %q is missing (the scanner accepts it)", f)
		}
	}
	if v := Video(); v[0] != "-protocol_whitelist" || len(v) != 4 {
		t.Errorf("Video() = %v", v)
	}
	if i := Image(); len(i) != 2 {
		t.Errorf("Image() = %v", i)
	}
}
