// Package subtitles serves subtitles to the player: text subtitles as
// WebVTT, ASS as it is (rendered by JASSUB or burned in by FFmpeg) and the
// fonts attached to the video.
package subtitles

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// Cue is one subtitle line on screen.
type Cue struct {
	Start, End time.Duration
	Settings   string // WebVTT cue settings ("line:10%"), with a leading space
	Text       string
}

// Decode returns subtitle bytes as UTF-8 text: a BOM (UTF-8, UTF-16) wins,
// then valid UTF-8; anything else is read as Windows-1252, the superset of
// Latin-1 that old Portuguese and Spanish subtitles use.
func Decode(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return string(b[3:])
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		return decodeUTF16(b[2:], binary.LittleEndian)
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		return decodeUTF16(b[2:], binary.BigEndian)
	case utf8.Valid(b):
		return string(b)
	}
	var sb strings.Builder
	sb.Grow(len(b) + len(b)/8)
	for _, c := range b {
		if c >= 0x80 && c <= 0x9F {
			sb.WriteRune(cp1252[c-0x80])
		} else {
			sb.WriteRune(rune(c)) // Latin-1 = the first 256 code points
		}
	}
	return sb.String()
}

func decodeUTF16(b []byte, order binary.ByteOrder) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = order.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}

// cp1252 maps the bytes 0x80-0x9F of Windows-1252 (undefined ones stay as is).
var cp1252 = [32]rune{
	'€', 0x81, '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', 0x8D, 'Ž', 0x8F,
	0x90, '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', 0x9D, 'ž', 'Ÿ',
}

var (
	timingRe = regexp.MustCompile(`^\s*((?:\d+:)?\d{1,2}:\d{1,2}[,.]\d{1,3})\s*-->\s*((?:\d+:)?\d{1,2}:\d{1,2}[,.]\d{1,3})(.*)$`)
	assTagRe = regexp.MustCompile(`\{\\[^}]*\}`)                                        // {\an8}, {\i1} in SRT files
	fontRe   = regexp.MustCompile(`(?i)</?font[^>]*>`)                                  // not in WebVTT
	tagRe    = regexp.MustCompile(`(?i)&lt;(/?)([ibu]|c(?:\.[\w.-]+)?)&gt;`)            // tags WebVTT keeps
	otherRe  = regexp.MustCompile(`(?i)&lt;/?(?:[a-z][\w.]*(?: [^&]*)?|\d[\d:.]*)&gt;`) // <v Bob>, <00:01.000>
)

// ParseCues reads SRT or WebVTT text (Decode it first). Blocks without a
// timing line (WEBVTT header, NOTE, STYLE) and empty cues are skipped.
func ParseCues(text string) []Cue {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	vtt := strings.HasPrefix(strings.TrimSpace(text), "WEBVTT")
	var cues []Cue
	var cur *Cue
	var lines []string
	flush := func() {
		if cur != nil {
			cur.Text = clean(strings.Join(lines, "\n"), vtt)
			if cur.Text != "" {
				cues = append(cues, *cur)
			}
		}
		cur, lines = nil, nil
	}
	collecting := false
	skipBlock := false
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			collecting, skipBlock = false, false
			continue
		}
		if vtt && !collecting && (trimmed == "NOTE" || strings.HasPrefix(trimmed, "NOTE ") || strings.HasPrefix(trimmed, "NOTE\t") || trimmed == "STYLE" || trimmed == "REGION") {
			skipBlock = true
		}
		if skipBlock {
			continue
		}
		if m := timingRe.FindStringSubmatch(line); m != nil {
			start, err1 := parseTime(m[1])
			end, err2 := parseTime(m[2])
			if err1 == nil && err2 == nil && end > start {
				// A cue counter glued to the previous text (no blank line) is not text.
				if n := len(lines); n > 0 && isDigits(lines[n-1]) {
					lines = lines[:n-1]
				}
				flush()
				cur = &Cue{Start: start, End: end}
				if vtt {
					cur.Settings = " " + strings.TrimSpace(m[3])
					if cur.Settings == " " {
						cur.Settings = ""
					}
				}
				collecting = true
				continue
			}
		}
		if collecting {
			lines = append(lines, line)
		}
	}
	flush()
	return cues
}

func isDigits(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// parseTime reads "01:02:03,456", "02:03.4" and similar.
func parseTime(s string) (time.Duration, error) {
	main, frac, _ := strings.Cut(strings.ReplaceAll(s, ",", "."), ".")
	parts := strings.Split(main, ":")
	var total time.Duration
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, fmt.Errorf("time %q: %w", s, err)
		}
		total = total*60 + time.Duration(n)
	}
	total *= time.Second
	for len(frac) < 3 {
		frac += "0" // ",5" is half a second
	}
	ms, err := strconv.Atoi(frac[:3])
	if err != nil {
		return 0, fmt.Errorf("time %q: %w", s, err)
	}
	return total + time.Duration(ms)*time.Millisecond, nil
}

// clean turns a cue text into WebVTT: ASS override tags, <font> and other
// tags go away, text is escaped and only <i>, <b>, <u> and <c> stay as tags.
func clean(s string, vtt bool) string {
	if vtt {
		s = html.UnescapeString(s) // escaped again below
	}
	s = assTagRe.ReplaceAllString(s, "")
	s = fontRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "-->", "→")
	s = html.EscapeString(s)
	s = strings.ReplaceAll(strings.ReplaceAll(s, "&#39;", "'"), "&#34;", `"`)
	s = tagRe.ReplaceAllStringFunc(s, func(t string) string {
		m := tagRe.FindStringSubmatch(t)
		return "<" + m[1] + strings.ToLower(m[2]) + ">"
	})
	return strings.TrimSpace(otherRe.ReplaceAllString(s, ""))
}

// WriteVTT renders cues as WebVTT on a timeline starting offset later (an
// HLS session that starts mid-file): cues that ended before it are dropped.
func WriteVTT(cues []Cue, offset time.Duration) []byte {
	var b bytes.Buffer
	b.WriteString("WEBVTT\n\n")
	for _, c := range cues {
		start, end := c.Start-offset, c.End-offset
		if end <= 0 {
			continue
		}
		fmt.Fprintf(&b, "%s --> %s%s\n%s\n\n", vttTime(max(start, 0)), vttTime(end), c.Settings, c.Text)
	}
	return b.Bytes()
}

func vttTime(d time.Duration) string {
	ms := d.Milliseconds()
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3_600_000, ms/60_000%60, ms/1000%60, ms%1000)
}
