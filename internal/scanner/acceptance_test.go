package scanner

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type messyCase struct {
	name    string
	kind    string
	title   string
	season  int
	episode int
}

func loadMessy(t *testing.T) []messyCase {
	t.Helper()
	f, err := os.Open("testdata/messy_library.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []messyCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fs := strings.Split(line, "\t")
		c := messyCase{name: fs[0], kind: fs[1]}
		if len(fs) > 2 {
			c.title = fs[2]
		}
		if len(fs) > 4 {
			c.season, _ = strconv.Atoi(fs[3])
			c.episode, _ = strconv.Atoi(fs[4])
		}
		out = append(out, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestMessyLibraryAcceptance: everything dumped in the root with release
// names must become a correct library for >= 95% of the files, and every
// file that is not correct must be in the review queue (never silently wrong).
func TestMessyLibraryAcceptance(t *testing.T) {
	cases := loadMessy(t)
	e := newEnv(t)
	for _, c := range cases {
		e.write(t, c.name)
	}
	e.scan(t)
	ctx := context.Background()

	correct, inReview, silentlyWrong := 0, 0, 0
	for _, c := range cases {
		var status, movieTitle, seriesTitle, seriesKind string
		var season, episode int
		err := e.db.QueryRowContext(ctx,
			`SELECT f.status, COALESCE(m.title, ''), COALESCE(s.title, ''), COALESCE(s.kind, ''),
			        COALESCE(se.number, -1), COALESCE(ep.number, -1)
			 FROM media_files f
			 LEFT JOIN movies m ON m.media_file_id = f.id
			 LEFT JOIN episodes ep ON ep.media_file_id = f.id
			 LEFT JOIN seasons se ON se.id = ep.season_id
			 LEFT JOIN series s ON s.id = ep.series_id
			 WHERE f.rel_path = ?`, c.name).Scan(&status, &movieTitle, &seriesTitle, &seriesKind, &season, &episode)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		ok := false
		got := ""
		switch c.kind {
		case "review":
			ok = status == "review"
			got = status
		case "movie":
			ok = movieTitle != "" && NormTitle(movieTitle) == NormTitle(c.title)
			got = "movie " + movieTitle
		default:
			ok = seriesKind == c.kind && NormTitle(seriesTitle) == NormTitle(c.title) &&
				season == c.season && episode == c.episode
			got = fmt.Sprintf("%s %q S%dE%d", seriesKind, seriesTitle, season, episode)
		}
		switch {
		case ok:
			correct++
		case status == "review":
			inReview++
			t.Logf("in review (acceptable): %s  [want %s %q]", c.name, c.kind, c.title)
		default:
			silentlyWrong++
			t.Errorf("silently wrong: %s → %s [want %s %q S%dE%d]", c.name, got, c.kind, c.title, c.season, c.episode)
		}
	}
	pct := 100 * float64(correct) / float64(len(cases))
	t.Logf("messy library: %d files, %d correct (%.1f%%), %d in review, %d silently wrong",
		len(cases), correct, pct, inReview, silentlyWrong)
	if pct < 95 {
		t.Errorf("accuracy %.1f%% < 95%%", pct)
	}

	// Grouping: one series per show, whatever the naming style.
	for _, show := range []string{"breaking bad", "game of thrones", "greys anatomy", "law and order svu", "cidade invisivel", "elite"} {
		if n := e.count(t, `SELECT count(*) FROM series WHERE norm_title = ?`, show); n != 1 {
			t.Errorf("series %q: %d rows, want 1", show, n)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM episodes e JOIN series s ON s.id = e.series_id WHERE s.norm_title = 'breaking bad'`); n != 8 {
		t.Errorf("Breaking Bad has %d episodes, want 8", n)
	}
}

// generateLibrary writes n files with a known expected outcome:
// 30% movies, 48% series episodes, 20% anime episodes, 2% samples/extras.
type libraryCounts struct {
	movies, series, anime, episodes, ignored, extras int
}

var words = []string{
	"Silent", "Crimson", "Hidden", "Broken", "Golden", "Lost", "Distant", "Frozen", "Burning", "Wild",
	"River", "Mountain", "Empire", "Garden", "Shadow", "Signal", "Harbor", "Orchard", "Station", "Lantern",
}

func title(i int) string {
	return words[i%10] + " " + words[10+(i/10)%10] + " " + words[(i/100)%10]
}

func generateLibrary(t testing.TB, e *env, n int) libraryCounts {
	t.Helper()
	var c libraryCounts
	movies, extras := n*30/100, n*2/100
	animeEps, seriesEps := n*20/100, n-n*30/100-n*20/100-n*2/100
	for i := range movies {
		y := 1950 + i%70
		if i%2 == 0 {
			name := fmt.Sprintf("Filmes/%s (%d)/%s (%d).mkv", title(i), y, title(i), y)
			e.write(t, name)
		} else {
			e.write(t, fmt.Sprintf("%s.%d.1080p.BluRay.x264-GRP.mkv", strings.ReplaceAll(title(i), " ", "."), y))
		}
		c.movies++
	}
	// extras/samples live in the first organized movie folders
	for i := range extras {
		m := (i / 2) * 2
		dir := fmt.Sprintf("Filmes/%s (%d)", title(m), 1950+m%70)
		if i%2 == 0 {
			e.write(t, dir+"/Sample/sample.mkv")
			c.ignored++
		} else {
			e.write(t, dir+"/Trailers/trailer.mkv")
			c.extras++
		}
	}
	// series: 12 episodes each, alternating naming styles
	for i := range seriesEps {
		show, ep := 500+i/12, i%12+1
		season := 1 + (ep-1)/6
		name := title(show)
		switch show % 3 {
		case 0:
			e.write(t, fmt.Sprintf("Series/%s/Season %d/%s - S%02dE%02d.mkv", name, season, name, season, ep))
		case 1:
			e.write(t, fmt.Sprintf("%s.S%02dE%02d.720p.WEB-DL.mkv", strings.ReplaceAll(name, " ", "."), season, ep))
		default:
			e.write(t, fmt.Sprintf("Downloads/%s %dx%02d.mkv", name, season, ep))
		}
		c.episodes++
	}
	c.series = (seriesEps + 11) / 12
	// anime: 10 episodes each
	for i := range animeEps {
		show, ep := 800+i/10, i%10+1
		e.write(t, fmt.Sprintf("[Group%d] %s - %02d [1080p].mkv", show%4, title(show), ep))
		c.episodes++
	}
	c.anime = (animeEps + 9) / 10
	return c
}

func TestThousandFileLibrary(t *testing.T) {
	e := newEnv(t)
	want := generateLibrary(t, e, 1000)
	st := e.scan(t)
	if st.Seen != 1000 || st.Added != 1000 {
		t.Fatalf("seen=%d added=%d", st.Seen, st.Added)
	}
	got := libraryCounts{
		movies:   e.count(t, `SELECT count(*) FROM movies`),
		series:   e.count(t, `SELECT count(*) FROM series WHERE kind = 'series'`),
		anime:    e.count(t, `SELECT count(*) FROM series WHERE kind = 'anime'`),
		episodes: e.count(t, `SELECT count(*) FROM episodes`),
		ignored:  e.count(t, `SELECT count(*) FROM media_files WHERE status = 'ignored'`),
		extras:   e.count(t, `SELECT count(*) FROM extras`),
	}
	if got != want {
		t.Fatalf("library = %+v, want %+v", got, want)
	}
	if n := e.count(t, `SELECT count(*) FROM media_files WHERE status = 'review'`); n != 0 {
		t.Fatalf("%d files in review, want 0", n)
	}
}

// BenchmarkRescanNoChanges measures the incremental scan of 10,000 files
// that did not change (Sprint 1 target: < 3 s).
func BenchmarkRescanNoChanges(b *testing.B) {
	e := newEnv(b)
	generateLibrary(b, e, 10000)
	if _, err := e.sc.Scan(context.Background()); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		st, err := e.sc.Scan(context.Background())
		if err != nil {
			b.Fatal(err)
		}
		if st.Unchanged != 10000 {
			b.Fatalf("unchanged = %d", st.Unchanged)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Milliseconds())/float64(b.N), "ms/scan")
}

// TestRescanUnderThreeSeconds runs the same check as the benchmark in the
// normal test suite, so a regression fails `make test`.
func TestRescanUnderThreeSeconds(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	e := newEnv(t)
	generateLibrary(t, e, 10000)
	e.scan(t)
	start := time.Now()
	st := e.scan(t)
	d := time.Since(start)
	t.Logf("incremental scan of %d unchanged files: %s", st.Unchanged, d)
	if st.Unchanged != 10000 || d > 3*time.Second {
		t.Fatalf("unchanged=%d in %s (want 10000 in < 3s)", st.Unchanged, d)
	}
}
