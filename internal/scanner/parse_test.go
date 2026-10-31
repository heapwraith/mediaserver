package scanner

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
)

type parseCase struct {
	line int
	path string
	want map[string]string
}

// loadParseCases reads testdata/parse_cases.txt (format documented there).
func loadParseCases(t *testing.T) []parseCase {
	t.Helper()
	f, err := os.Open("testdata/parse_cases.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var cases []parseCase
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		c := parseCase{line: n, path: fields[0], want: map[string]string{}}
		for _, kv := range fields[1:] {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				t.Fatalf("line %d: bad field %q", n, kv)
			}
			c.want[k] = v
		}
		if _, ok := c.want["kind"]; !ok {
			t.Fatalf("line %d: missing kind", n)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return cases
}

func TestParseCorpus(t *testing.T) {
	cases := loadParseCases(t)
	if len(cases) < 150 {
		t.Fatalf("corpus has %d cases, Sprint 1 requires at least 150", len(cases))
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			p := Parse(c.path)
			cl := Classify(p)
			got := map[string]string{
				"kind":   string(cl.Kind),
				"title":  p.Title,
				"year":   itoa(p.Year),
				"s":      itoa(p.Season),
				"e":      itoa(p.Episode),
				"e2":     itoa(p.EpisodeEnd),
				"abs":    itoa(p.Absolute),
				"v":      itoa(p.Version),
				"group":  p.Group,
				"et":     p.EpisodeTitle,
				"extra":  p.ExtraType,
				"own":    p.OwnerDir,
				"reason": cl.Reason,
			}
			// Unlisted numbers must be zero; unlisted strings are not checked
			// (except kind and title, which every line lists).
			for _, k := range []string{"year", "s", "e", "e2", "abs", "v"} {
				if _, ok := c.want[k]; !ok {
					c.want[k] = "0"
				}
			}
			for k, want := range c.want {
				if got[k] != want {
					t.Errorf("line %d: %s = %q, want %q", c.line, k, got[k], want)
				}
			}
		})
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestNormTitle(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Amélie", "amelie"},
		{"Cidade de Deus", "cidade de deus"},
		{"CIDADE.DE.DEUS", "cidade de deus"},
		{"Grey's Anatomy", "greys anatomy"},
		{"Law & Order: SVU", "law and order svu"},
		{"Spider-Man", "spider man"},
		{"Ação, Ção & Coração!", "acao cao and coracao"},
		{"Øresund Brücke", "oresund brucke"},
		{"葬送のフリーレン", "葬送のフリーレン"},
		{"ブルーロック", "ブルーロック"}, // dakuten kept
		{"  Spaced   Out  ", "spaced out"},
		{"Steins;Gate 0", "steins gate 0"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := NormTitle(tt.in); got != tt.want {
				t.Errorf("NormTitle(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
