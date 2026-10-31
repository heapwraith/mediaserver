package stream

import (
	"slices"
	"strconv"

	"mediaserver/internal/library"
	"mediaserver/internal/probe"
)

// Plan is how one client plays one media file (AGENTS.md §5.1): the original
// file, or HLS with each stream copied or converted.
type Plan struct {
	Direct bool
	// HLS only.
	Video      string // "copy" or "h264" (encode)
	VideoCodec string // source codec, for the copy (hevc needs the hvc1 tag)
	Audio      string // "copy", "aac" (encode) or "" (no audio)
	AudioIndex int    // ffprobe stream index of the audio track, -1 = none
	Burn       string // ASS file drawn into the video (Burn.File), "" = none
}

// Streams that fMP4 HLS carries as they are (hls.js and Safari read them).
var (
	copyVideo = map[string]bool{"h264": true, "hevc": true, "av1": true}
	copyAudio = map[string]bool{"aac": true, "ac3": true, "eac3": true, "mp3": true}
)

// Decide picks direct play → remux → transcoding from the tokens the client
// declares (web/src/lib/platform/codecs.ts), never from its user agent:
// containers mp4, webm, mkv, mov; video h264, hevc, av1, vp9 (":10" = 10-bit);
// audio aac, mp3, opus, flac, ac3, eac3. audio is the ffprobe index of the
// wanted audio track (another value = the default track). burn: a subtitle
// is drawn into the video, so it is always encoded.
func Decide(m library.Media, caps []string, audio int, burn bool) Plan {
	has := func(token string) bool { return token != "" && slices.Contains(caps, token) }
	track, isDefault := audioTrack(m.Audio, audio)

	videoOK := m.VideoCodec == "" || has(videoToken(m.VideoCodec, m.VideoBits))
	audioOK := track == nil || has(audioToken(track.Codec))
	// Direct play plays the default track: another one needs HLS.
	if !burn && has(containerToken(m.Container)) && videoOK && audioOK && isDefault {
		return Plan{Direct: true, AudioIndex: -1}
	}

	p := Plan{Video: "h264", VideoCodec: m.VideoCodec, AudioIndex: -1}
	if !burn && videoOK && (m.VideoCodec == "" || copyVideo[m.VideoCodec]) {
		p.Video = "copy"
	}
	if track != nil {
		p.AudioIndex = track.Index
		p.Audio = "aac"
		if audioOK && copyAudio[track.Codec] {
			p.Audio = "copy"
		}
	}
	return p
}

// audioTrack returns the track with ffprobe index, or the default one, and
// whether it is the track a browser plays by default.
func audioTrack(tracks []probe.Track, index int) (*probe.Track, bool) {
	if len(tracks) == 0 {
		return nil, true
	}
	def := 0
	for i, t := range tracks {
		if t.Default {
			def = i
			break
		}
	}
	for i := range tracks {
		if tracks[i].Index == index {
			return &tracks[i], i == def
		}
	}
	return &tracks[def], true
}

func containerToken(c string) string {
	switch c {
	case "mp4", "m4v":
		return "mp4"
	case "webm", "mkv", "mov":
		return c
	}
	return ""
}

func videoToken(codec string, bits int) string {
	switch codec {
	case "h264", "hevc", "av1", "vp9":
		if bits > 8 {
			return codec + ":" + strconv.Itoa(bits)
		}
		return codec
	}
	return ""
}

func audioToken(codec string) string {
	switch codec {
	case "aac", "mp3", "opus", "flac", "ac3", "eac3":
		return codec
	}
	return ""
}
