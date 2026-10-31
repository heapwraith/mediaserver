package scanner

import (
	"regexp"
	"strings"
)

// Video and sidecar extensions (AGENTS.md §4).
var (
	videoExts    = setOf(".mkv", ".mp4", ".m4v", ".avi", ".webm", ".mov", ".ts")
	subtitleExts = setOf(".srt", ".ass", ".ssa", ".vtt")
	imageExts    = setOf(".jpg", ".jpeg", ".png", ".webp")
)

// IsVideo reports whether name has a video extension.
func IsVideo(name string) bool { return videoExts[strings.ToLower(extOf(name))] }

// IsSubtitle reports whether name has a sidecar subtitle extension.
func IsSubtitle(name string) bool { return subtitleExts[strings.ToLower(extOf(name))] }

// IsImage reports whether name has an image extension.
func IsImage(name string) bool { return imageExts[strings.ToLower(extOf(name))] }

func extOf(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i:]
	}
	return ""
}

func setOf(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it] = true
	}
	return m
}

// strongTags end the title: nothing after them is part of the name.
// Lowercase, compared token by token (separators already turned into spaces).
var strongTags = setOf(
	// resolution
	"4k", "uhd", "fhd", "sd",
	// codec
	"x264", "x265", "h264", "h265", "hevc", "avc", "av1", "xvid", "divx", "vp9",
	"10bit", "8bit", "12bit", "hi10p", "hi10",
	// source
	"bluray", "blu-ray", "bdrip", "brrip", "bdremux", "remux", "bd", "bdmv",
	"web-dl", "webdl", "webrip", "web-rip", "hdtv", "hdrip", "dvdrip", "dvdscr", "dvd", "dvd9", "dvd5",
	"hdtvrip", "tvrip", "vhsrip", "hdcam", "telesync", "ts-rip",
	// audio
	"dts", "dts-hd", "dtshd", "dts-x", "truehd", "atmos", "aac", "ac3", "eac3", "flac", "opus", "mp3",
	"ddp", "dd", "dd+", "lpcm", "pcm",
	// release flags (AGENTS.md §4)
	"repack", "proper", "rerip",
)

// weakTags are dropped when they appear after the title, but never end it
// on their own ("Charlotte's Web" must survive).
var weakTags = setOf(
	"web", "hdr", "hdr10", "hdr10+", "dv", "dovi", "sdr", "imax", "hybrid",
	"extended", "unrated", "uncut", "remastered", "directors", "internal", "limited",
	"multi", "dual", "dubbed", "subbed", "dublado", "legendado", "nacional", "multisub",
	"amzn", "nf", "dsnp", "hmax", "atvp", "hulu", "pcok", "cr", "funi", "adn",
	"complete", "batch",
	// audio language flags
	"korean", "german", "french", "truefrench", "japanese", "italian", "spanish", "latino",
	"castellano", "portuguese", "english", "hindi", "russian", "chinese", "vff", "vfq", "vostfr",
)

var (
	reResolution = regexp.MustCompile(`^\d{3,4}[pi]$`)
	reAudioCh    = regexp.MustCompile(`^(?:ddp?|eac3|ac3|aac|dts|truehd|flac|opus)\d(?:[.]\d)?$`)
	reCodecDot   = regexp.MustCompile(`^[hx]26[45]$`)
	reHash       = regexp.MustCompile(`^[0-9a-fA-F]{8}$`)
	reYear       = regexp.MustCompile(`^(?:19|20)\d{2}$`)
)

// isStrongTag reports whether tok (lowercase) ends a title.
func isStrongTag(tok string) bool {
	if strongTags[tok] || reResolution.MatchString(tok) || reAudioCh.MatchString(tok) || reCodecDot.MatchString(tok) {
		return true
	}
	// "x264-GROUP", "WEB-DL-GROUP": the part before the dash decides.
	if before, _, ok := strings.Cut(tok, "-"); ok && before != "" {
		return strongTags[before] || reResolution.MatchString(before) || reCodecDot.MatchString(before)
	}
	return false
}

// isReleaseTag reports whether tok (lowercase) is any kind of release tag.
func isReleaseTag(tok string) bool {
	return isStrongTag(tok) || weakTags[tok]
}

// isTagGroup reports whether every token of a bracket content is a release
// tag, a hash or a known noise word (so the whole group can be dropped).
func isTagGroup(content string) bool {
	toks := strings.FieldsFunc(strings.ToLower(content), func(r rune) bool {
		return r == ' ' || r == '.' || r == '_' || r == ',' || r == '+' || r == '/'
	})
	if len(toks) == 0 {
		return true
	}
	for _, t := range toks {
		if !isReleaseTag(t) && !reHash.MatchString(t) && !noiseWords[t] && !isNumberish(t) {
			return false
		}
	}
	return true
}

// noiseWords appear inside bracket groups of anime releases.
var noiseWords = setOf(
	"eng", "sub", "subs", "esub", "msubs", "jap", "jpn", "ita", "pt-br", "ptbr", "por", "spa", "fre",
	"vostfr", "raw", "raws", "end", "fin", "final", "ch", "audio", "dual-audio", "multi-subs",
	"bd", "web", "tv", "uncensored", "censored", "softsub", "hardsub", "x264", "aac2", "opus2",
)

func isNumberish(t string) bool {
	if t == "" {
		return false
	}
	for _, r := range t {
		if (r < '0' || r > '9') && r != '.' && r != 'x' && r != 'v' {
			return false
		}
	}
	return true
}
