package subtitles

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func utf16LE(s string) []byte {
	b := []byte{0xFF, 0xFE}
	for _, u := range utf16.Encode([]rune(s)) {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	return b
}

func TestDecode(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"utf-8", []byte("ação, você"), "ação, você"},
		{"utf-8 bom", append([]byte{0xEF, 0xBB, 0xBF}, "ação"...), "ação"},
		{"utf-16le bom", utf16LE("você é 日本"), "você é 日本"},
		// "ação, você" saved by an old editor in Latin-1.
		{"latin-1", []byte{'a', 0xE7, 0xE3, 'o', ',', ' ', 'v', 'o', 'c', 0xEA}, "ação, você"},
		{"windows-1252 quotes", []byte{0x93, 'O', 'i', 0x94, ' ', 0x96, ' ', 0x80, '5'}, "“Oi” – €5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decode(tt.in); got != tt.want {
				t.Errorf("Decode = %q, want %q", got, tt.want)
			}
		})
	}
}

const srt = "1\r\n00:00:01,000 --> 00:00:02,500\r\n<i>Ação</i> & <font color=\"#ff0\">você</font>\r\n\r\n" +
	"2\r\n00:00:03,000 --> 00:00:04,000\r\n{\\an8}No topo\r\nsegunda linha\r\n" +
	"3\r\n00:01:05,5 --> 00:01:06,75\r\n<B>sem linha em branco</B> <v Bob>x</v>\r\n\r\n" +
	"4\r\n00:00:07,000 --> 00:00:08,000\r\n\r\n" // empty cue

func TestParseSRT(t *testing.T) {
	cues := ParseCues(srt)
	want := []Cue{
		{Start: time.Second, End: 2500 * time.Millisecond, Text: "<i>Ação</i> &amp; você"},
		{Start: 3 * time.Second, End: 4 * time.Second, Text: "No topo\nsegunda linha"},
		{Start: 65500 * time.Millisecond, End: 66750 * time.Millisecond, Text: "<b>sem linha em branco</b> x"},
	}
	if len(cues) != len(want) {
		t.Fatalf("cues = %+v", cues)
	}
	for i := range want {
		if cues[i] != want[i] {
			t.Errorf("cue %d = %+v, want %+v", i, cues[i], want[i])
		}
	}
}

func TestParseVTT(t *testing.T) {
	vtt := "WEBVTT - legenda\n\nSTYLE\n::cue { color: yellow }\n\nNOTE comentário\n\n" +
		"intro\n00:01.000 --> 00:02.000 line:10% align:start\nOlá &amp; <c.yellow>tchau</c> <00:01.500>fim\n\n" +
		"01:00:00.000 --> 01:00:01.000\numa hora\n"
	cues := ParseCues(vtt)
	if len(cues) != 2 {
		t.Fatalf("cues = %+v", cues)
	}
	if c := cues[0]; c.Start != time.Second || c.Settings != " line:10% align:start" ||
		c.Text != "Olá &amp; <c.yellow>tchau</c> fim" {
		t.Errorf("cue 0 = %+v", c)
	}
	if c := cues[1]; c.Start != time.Hour || c.Text != "uma hora" {
		t.Errorf("cue 1 = %+v", c)
	}
}

func TestVTTIgnoredBlocks(t *testing.T) {
	for _, block := range []string{"NOTE", "NOTE comment", "NOTE\tcomment", "STYLE", "REGION"} {
		t.Run(block, func(t *testing.T) {
			text := "WEBVTT\n\n" + block + "\n00:01.000 --> 00:02.000\nnot a cue\n\n00:03.000 --> 00:04.000\n123\n\n"
			cues := ParseCues(text)
			if len(cues) != 1 || cues[0].Text != "123" || cues[0].Start != 3*time.Second {
				t.Fatalf("cues = %+v", cues)
			}
		})
	}
}

func TestWriteVTT(t *testing.T) {
	cues := []Cue{
		{Start: time.Second, End: 2 * time.Second, Text: "antes"},
		{Start: 9 * time.Second, End: 12 * time.Second, Text: "cortada"},
		{Start: 3723 * time.Second, End: 3724500 * time.Millisecond, Settings: " line:0", Text: "depois"},
	}
	want := "WEBVTT\n\n" +
		"00:00:00.000 --> 00:00:02.000\ncortada\n\n" +
		"01:01:53.000 --> 01:01:54.500 line:0\ndepois\n\n"
	if got := string(WriteVTT(cues, 10*time.Second)); got != want {
		t.Errorf("WriteVTT =\n%s\nwant\n%s", got, want)
	}
	if got := string(WriteVTT(nil, 0)); !strings.HasPrefix(got, "WEBVTT\n") {
		t.Errorf("empty = %q", got)
	}
}
