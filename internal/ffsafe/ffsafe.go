// SPDX-License-Identifier: GPL-3.0-or-later

// Package ffsafe holds the FFmpeg/ffprobe input options that keep a hostile
// media file (a crafted playlist, concat script or odd container) from making
// the tools read other files, open network connections or run an unexpected
// demuxer. The files are the user's, but they often come from third parties.
package ffsafe

// Protocols is the only I/O FFmpeg may use for an input: local files and pipes.
// No http, tcp, udp, rtmp, ftp, data or subfile: a media file cannot reach the network.
const Protocols = "file,pipe"

// VideoFormats are the demuxers of the containers the scanner accepts
// (.mkv .mp4 .m4v .avi .webm .mov .ts). An HLS, DASH or concat playlist
// renamed to .mkv is refused instead of followed.
const VideoFormats = "matroska,webm,mov,mp4,m4a,3gp,3g2,mj2,avi,mpegts"

// Image returns the options for an image input (poster, fanart, frame).
func Image() []string { return []string{"-protocol_whitelist", Protocols} }

// Video returns the options for a video input; put them before the "-i".
func Video() []string {
	return []string{"-protocol_whitelist", Protocols, "-format_whitelist", VideoFormats}
}
