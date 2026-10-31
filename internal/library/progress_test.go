package library

import (
	"fmt"
	"testing"
	"time"
	"uuid"
)

func TestIsWatched(t *testing.T) {
	tests := []struct {
		pos, dur int64
		want     bool
	}{
		{0, 0, false},
		{100, 0, false},
		{899, 1000, false},
		{900, 1000, true},
		{1000, 1000, true},
	}
	for _, tt := range tests {
		if got := IsWatched(tt.pos, tt.dur); got != tt.want {
			t.Errorf("IsWatched(%d, %d) = %v", tt.pos, tt.dur, got)
		}
	}
}

func TestPIN(t *testing.T) {
	h, err := HashPIN("0420")
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := HashPIN("0420")
	if h == h2 {
		t.Error("same hash twice: the salt is missing")
	}
	for pin, want := range map[string]bool{"0420": true, "0421": false, "": false, "04200": false} {
		if VerifyPIN(h, pin) != want {
			t.Errorf("VerifyPIN(%q) = %v", pin, !want)
		}
	}
	for _, bad := range []string{"", "plain", "pbkdf2-sha256$x$00$00", "md5$1$00$00"} {
		if VerifyPIN(bad, "0420") {
			t.Errorf("VerifyPIN accepted hash %q", bad)
		}
	}
}

func TestUpNext(t *testing.T) {
	order := []epRef{{mediaID: "e1"}, {mediaID: "e2"}, {mediaID: "e3"}, {mediaID: "sp1", special: true}}
	p := func(at int64, watched bool) Progress { return Progress{UpdatedAt: at, Watched: watched} }
	tests := []struct {
		name string
		prog map[string]Progress
		want string
	}{
		{"nothing played", map[string]Progress{}, ""},
		{"in progress", map[string]Progress{"e1": p(1, true), "e2": p(2, false)}, "e2"},
		{"finished: the next one", map[string]Progress{"e1": p(1, true)}, "e2"},
		{"skips watched ones", map[string]Progress{"e1": p(2, true), "e2": p(1, true)}, "e3"},
		{"last regular: no special", map[string]Progress{"e3": p(1, true)}, "e1"},
		{"all watched", map[string]Progress{"e1": p(1, true), "e2": p(1, true), "e3": p(3, true)}, ""},
		{"latest wins over order", map[string]Progress{"e3": p(1, false), "e1": p(5, true)}, "e2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := upNext(order, tt.prog); got != tt.want {
				t.Errorf("upNext = %q, want %q", got, tt.want)
			}
		})
	}
}

// seedSeries adds n series of 2 seasons × eps episodes (plus a special) to s,
// returning the episode media ids of each series in viewing order.
func seedSeries(tb testing.TB, s Store, n, eps int) [][]string {
	tb.Helper()
	ctx := tb.Context()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		tb.Fatal(err)
	}
	exec := func(q string, args ...any) {
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			tb.Fatal(err)
		}
	}
	var lib string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM libraries LIMIT 1`).Scan(&lib); err != nil {
		tb.Fatal(err)
	}
	now := time.Now().UnixMilli()
	out := make([][]string, n)
	for i := range n {
		sid := uuid.NewV7().String()
		title := fmt.Sprintf("Series %04d", i)
		exec(`INSERT INTO series (id, kind, title, sort_title, norm_title, added_at, updated_at) VALUES (?, 'series', ?, ?, ?, ?, ?)`,
			sid, title, title, title, now, now)
		var special []string
		for _, season := range []int{0, 1, 2} {
			seid := uuid.NewV7().String()
			exec(`INSERT INTO seasons (id, series_id, number) VALUES (?, ?, ?)`, seid, sid, season)
			count := eps
			if season == 0 {
				count = 1
			}
			for e := 1; e <= count; e++ {
				mf := uuid.NewV7().String()
				exec(`INSERT INTO media_files (id, library_id, rel_path, size, mtime_ns, kind, duration_ms, added_at, updated_at)
				      VALUES (?, ?, ?, 1, 1, 'episode', 1200000, ?, ?)`, mf, lib, fmt.Sprintf("%s/S%02dE%02d.mkv", title, season, e), now, now)
				exec(`INSERT INTO episodes (id, series_id, season_id, media_file_id, number, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
					uuid.NewV7().String(), sid, seid, mf, e, now)
				if season == 0 {
					special = append(special, mf)
				} else {
					out[i] = append(out[i], mf)
				}
			}
		}
		out[i] = append(out[i], special...)
	}
	if err := tx.Commit(); err != nil {
		tb.Fatal(err)
	}
	return out
}

func TestProgressStore(t *testing.T) {
	s := seedMovies(t, 3)
	ctx := t.Context()
	series := seedSeries(t, s, 2, 3)
	ana, err := s.CreateProfile(ctx, ProfileInput{Name: "Ana", Color: "#3E63DD"})
	if err != nil {
		t.Fatal(err)
	}
	if ana.Color != "#3e63dd" {
		t.Errorf("color not normalized: %q", ana.Color)
	}
	movies, err := s.Movies(ctx, Page{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	movie := movies.Items[0].MediaID
	a := series[0] // S1E1..E3, S2E1..E3, special

	// Watched is sticky: going back to the start keeps it.
	if p, err := s.SaveProgress(ctx, ana.ID, a[0], 1_150_000, 0); err != nil || !p.Watched {
		t.Fatalf("save = %+v %v", p, err)
	}
	if p, _ := s.SaveProgress(ctx, ana.ID, a[0], 10_000, 0); !p.Watched || p.PositionMs != 10_000 {
		t.Fatalf("sticky watched = %+v", p)
	}
	if _, err := s.SaveProgress(ctx, ana.ID, uuid.NewV7().String(), 1, 1); err == nil {
		t.Error("unknown media saved")
	}
	if _, err := s.SaveProgress(ctx, ana.ID, movie, -1, 0); err == nil {
		t.Error("negative position saved")
	}

	// Below 30 s an item is not "started".
	if _, err := s.SaveProgress(ctx, ana.ID, movie, 20_000, 0); err != nil {
		t.Fatal(err)
	}
	h, err := s.HomeOf(ctx, ana.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Continue) != 0 || len(h.Next) != 1 || h.Next[0].MediaID != a[1] || h.Next[0].Season != 1 {
		t.Fatalf("home = %+v", h)
	}

	// Two episodes in progress: the series shows once, with the latest.
	time.Sleep(2 * time.Millisecond)
	if _, err := s.SaveProgress(ctx, ana.ID, a[3], 60_000, 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := s.SaveProgress(ctx, ana.ID, a[4], 90_000, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveProgress(ctx, ana.ID, movie, 600_000, 0); err != nil {
		t.Fatal(err)
	}
	h, _ = s.HomeOf(ctx, ana.ID)
	if len(h.Continue) != 2 || h.Continue[0].MediaID != movie || h.Continue[1].MediaID != a[4] ||
		h.Continue[1].Episode == nil || h.Continue[1].Season != 2 || len(h.Next) != 0 {
		t.Fatalf("home = %+v", h)
	}

	// Marking a season watched moves "next" on; the last regular episode
	// does not chain into the specials.
	var season2 string
	if err := s.DB.QueryRowContext(ctx, `SELECT season_id FROM episodes WHERE media_file_id = ?`, a[3]).Scan(&season2); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWatched(ctx, ana.ID, WatchedTarget{SeasonID: season2, Watched: true}); err != nil {
		t.Fatal(err)
	}
	sp, err := s.SeriesProgressOf(ctx, ana.ID, mustSeriesOf(t, s, a[0]))
	if err != nil {
		t.Fatal(err)
	}
	if sp.UpNext != a[1] || sp.Episodes[a[4]].PositionMs != 0 || !sp.Episodes[a[5]].Watched {
		t.Fatalf("series progress = %+v", sp)
	}
	if next, err := s.NextEpisode(ctx, a[5]); err == nil {
		t.Errorf("next of the last regular episode = %+v, want none", next)
	}
	if next, err := s.NextEpisode(ctx, a[2]); err != nil || next.MediaID != a[3] || next.Episode.Number != 1 {
		t.Errorf("next across seasons = %+v %v", next, err)
	}
	if err := s.SetWatched(ctx, ana.ID, WatchedTarget{SeriesID: uuid.NewV7().String(), Watched: true}); err == nil {
		t.Error("unknown series marked")
	}

	// Deleting the profile deletes its progress.
	if err := s.DeleteProfile(ctx, ana.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM progress`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("progress left = %d %v", left, err)
	}
}

func mustSeriesOf(t *testing.T, s Store, mediaID string) string {
	t.Helper()
	var id string
	if err := s.DB.QueryRowContext(t.Context(), `SELECT series_id FROM episodes WHERE media_file_id = ?`, mediaID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestProfilesStore(t *testing.T) {
	s := seedMovies(t, 0)
	ctx := t.Context()
	pin := "1234"
	p, err := s.CreateProfile(ctx, ProfileInput{Name: "  Bia ", Color: "#112233", PIN: &pin})
	if err != nil || p.Name != "Bia" || !p.HasPIN {
		t.Fatalf("create = %+v %v", p, err)
	}
	for _, in := range []ProfileInput{
		{Name: "", Color: "#112233"},
		{Name: "Abcdefghijklmnopqrstu", Color: "#112233"},
		{Name: "X", Color: "#12345"},
		{Name: "X", Color: "#112233", PIN: new("12a4")},
	} {
		if _, err := s.CreateProfile(ctx, in); err == nil {
			t.Errorf("created invalid %+v", in)
		}
	}
	// nil PIN keeps it; "" removes it.
	if p, err = s.UpdateProfile(ctx, p.ID, ProfileInput{Name: "Bia", Color: "#112233"}); err != nil || !p.HasPIN {
		t.Fatalf("update keeping pin = %+v %v", p, err)
	}
	if p, err = s.UpdateProfile(ctx, p.ID, ProfileInput{Name: "Bia", Color: "#112233", PIN: new("")}); err != nil || p.HasPIN {
		t.Fatalf("update removing pin = %+v %v", p, err)
	}
	for i := 1; i < MaxProfiles; i++ {
		if _, err := s.CreateProfile(ctx, ProfileInput{Name: fmt.Sprintf("P%d", i), Color: "#112233"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateProfile(ctx, ProfileInput{Name: "Extra", Color: "#112233"}); err == nil {
		t.Errorf("created more than %d profiles", MaxProfiles)
	}
	secret, err := s.Secret(ctx)
	again, _ := s.Secret(ctx)
	if err != nil || len(secret) != 32 || string(secret) != string(again) {
		t.Fatalf("secret not stable: %v", err)
	}
}

// BenchmarkHome measures the profile rows of the home page with 10k movies,
// 200 series and 500 progress rows (AGENTS.md §8: listing < 50 ms p95).
func BenchmarkHome(b *testing.B) {
	s := seedMovies(b, 10_000)
	ctx := b.Context()
	series := seedSeries(b, s, 200, 10)
	p, err := s.CreateProfile(ctx, ProfileInput{Name: "Bench", Color: "#112233"})
	if err != nil {
		b.Fatal(err)
	}
	movies, err := s.Movies(ctx, Page{Limit: 300})
	if err != nil {
		b.Fatal(err)
	}
	for i, m := range movies.Items {
		if _, err := s.SaveProgress(ctx, p.ID, m.MediaID, int64(60_000+i), 0); err != nil {
			b.Fatal(err)
		}
	}
	for i, eps := range series {
		// Two finished episodes per series; every 4th one is mid-episode.
		for j := range 2 {
			if _, err := s.SaveProgress(ctx, p.ID, eps[j], 1_200_000, 0); err != nil {
				b.Fatal(err)
			}
		}
		if i%4 == 0 {
			if _, err := s.SaveProgress(ctx, p.ID, eps[2], 120_000, 0); err != nil {
				b.Fatal(err)
			}
		}
	}
	for b.Loop() {
		if _, err := s.HomeOf(ctx, p.ID); err != nil {
			b.Fatal(err)
		}
	}
}
