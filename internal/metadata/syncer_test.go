package metadata

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"uuid"

	"mediaserver/internal/db"
	"mediaserver/internal/scanner"
	"mediaserver/migrations"
)

const testKey = "abc123secretkey"

// fakeTMDB answers from routes ("/movie/10" → JSON) and records every call.
type fakeTMDB struct {
	mu     sync.Mutex
	calls  []string
	routes map[string]string
	status int // when set, every call answers this
}

func (f *fakeTMDB) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.URL.Path)
	switch {
	case f.status != 0:
		w.WriteHeader(f.status)
		return
	case r.URL.Query().Get("language") != language:
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	body, ok := f.routes[r.URL.Path+"?"+r.URL.Query().Get("query")]
	if !ok {
		body, ok = f.routes[r.URL.Path]
	}
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_, _ = w.Write([]byte(body))
}

func (f *fakeTMDB) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type env struct {
	t    *testing.T
	s    *Syncer
	db   *sql.DB
	fake *fakeTMDB
	srv  *httptest.Server
}

func newEnv(t *testing.T) *env {
	t.Helper()
	conn, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := db.Migrate(t.Context(), conn, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(t.Context(), `INSERT INTO libraries (id, name, root_path, created_at) VALUES ('lib', 'm', '/m', 0)`); err != nil {
		t.Fatal(err)
	}
	fake := &fakeTMDB{routes: map[string]string{}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	s := New(conn, slog.New(slog.DiscardHandler))
	s.base, s.pace = srv.URL, 0
	return &env{t: t, s: s, db: conn, fake: fake, srv: srv}
}

func (e *env) exec(q string, args ...any) {
	e.t.Helper()
	if _, err := e.db.ExecContext(e.t.Context(), q, args...); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) file(tags string) string {
	e.t.Helper()
	id := uuid.NewV7().String()
	raw := `{"streams":[{"codec_type":"video","codec_name":"h264"}],"format":{"format_name":"matroska","tags":{` + tags + `}}}`
	e.exec(`INSERT INTO media_files (id, library_id, rel_path, size, mtime_ns, added_at, updated_at, probe_json)
	        VALUES (?, 'lib', ?, 1, 1, 1, 1, ?)`, id, id+".mkv", raw)
	return id
}

func (e *env) movie(name string, year int, tags string) string {
	e.t.Helper()
	id := uuid.NewV7().String()
	e.exec(`INSERT INTO movies (id, media_file_id, title, sort_title, norm_title, year, added_at) VALUES (?, ?, ?, ?, ?, ?, 1)`,
		id, e.file(tags), name, name, scanner.NormTitle(name), year)
	return id
}

func (e *env) series(name, tags string) string {
	e.t.Helper()
	id, season := uuid.NewV7().String(), uuid.NewV7().String()
	e.exec(`INSERT INTO series (id, kind, title, sort_title, norm_title, added_at, updated_at) VALUES (?, 'series', ?, ?, ?, 1, 1)`,
		id, name, name, scanner.NormTitle(name))
	e.exec(`INSERT INTO seasons (id, series_id, number) VALUES (?, ?, 1)`, season, id)
	for n, t := range []string{``, tags} { // the tag is only in the second episode
		e.exec(`INSERT INTO episodes (id, series_id, season_id, media_file_id, number, added_at) VALUES (?, ?, ?, ?, ?, 1)`,
			uuid.NewV7().String(), id, season, e.file(t), n+1)
	}
	return id
}

func (e *env) turnOn() {
	e.t.Helper()
	if err := e.s.SetKey(e.t.Context(), testKey); err != nil {
		e.t.Fatal(err)
	}
	if err := e.s.SetEnabled(e.t.Context(), true); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) str(q string, args ...any) string {
	e.t.Helper()
	var v sql.NullString
	if err := e.db.QueryRowContext(e.t.Context(), q, args...).Scan(&v); err != nil && !errors.Is(err, sql.ErrNoRows) {
		e.t.Fatal(err)
	}
	return v.String
}

func (e *env) status(id string) string {
	return e.str(`SELECT status FROM title_meta WHERE owner_id = ?`, id)
}

func (e *env) genres(id string) string {
	return e.str(`SELECT group_concat(g.name, ',') FROM title_genres tg JOIN genres g ON g.id = tg.genre_id WHERE tg.owner_id = ?`, id)
}

func TestOffMakesNoCalls(t *testing.T) {
	e := newEnv(t)
	e.movie("A Origem", 2010, ``)
	if err := e.s.Sync(t.Context()); err != nil { // off and no key
		t.Fatal(err)
	}
	if err := e.s.SetKey(t.Context(), testKey); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Sync(t.Context()); err != nil { // a key, still off
		t.Fatal(err)
	}
	if err := e.s.SetKey(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	if err := e.s.SetEnabled(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Sync(t.Context()); err != nil { // on, without a key
		t.Fatal(err)
	}
	if n := e.fake.count(); n != 0 {
		t.Fatalf("TMDB called %d times: %v", n, e.fake.calls)
	}
}

func TestSearchWithOneAnswer(t *testing.T) {
	e := newEnv(t)
	id := e.movie("A Origem", 2010, ``)
	e.fake.routes["/search/movie?A Origem"] = `{"results":[
		{"id":27205,"title":"A Origem","original_title":"Inception","release_date":"2010-07-15"},
		{"id":99,"title":"A Origem do Mal","release_date":"2010-01-01"},
		{"id":98,"title":"A Origem","release_date":"1985-01-01"}]}`
	e.fake.routes["/movie/27205"] = `{"genres":[{"id":28,"name":"Ação"},{"id":878,"name":"Ficção científica"}],
		"belongs_to_collection":{"id":7,"name":"Coleção Origem"}}`
	e.turnOn()
	if err := e.s.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := e.status(id); got != "matched" {
		t.Fatalf("status = %q", got)
	}
	if got := e.genres(id); got != "Ação,Ficção científica" && got != "Ficção científica,Ação" {
		t.Errorf("genres = %q", got)
	}
	if got := e.str(`SELECT c.name FROM collection_movies m JOIN collections c ON c.id = m.collection_id WHERE m.movie_id = ?`, id); got != "Coleção Origem" {
		t.Errorf("collection = %q", got)
	}
	// Looked up once: the next scan asks nothing.
	before := e.fake.count()
	if err := e.s.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	if e.fake.count() != before {
		t.Errorf("second sync called TMDB: %v", e.fake.calls)
	}
	// A new title for the same file is looked up again.
	e.exec(`UPDATE movies SET title = 'Tenet', norm_title = 'tenet', year = 2020 WHERE id = ?`, id)
	e.fake.routes["/search/movie?Tenet"] = `{"results":[{"id":5,"title":"Tenet","release_date":"2020-08-22"}]}`
	e.fake.routes["/movie/5"] = `{"genres":[{"id":53,"name":"Suspense"}],"belongs_to_collection":null}`
	if err := e.s.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := e.genres(id); got != "Suspense" {
		t.Errorf("genres after the new title = %q", got)
	}
	if got := e.str(`SELECT count(*) FROM collection_movies WHERE movie_id = ?`, id); got != "0" {
		t.Errorf("old collection kept: %s", got)
	}
}

func TestTags(t *testing.T) {
	e := newEnv(t)
	bear := e.series("O Urso", `"TMDB":"tv/136315","IMDB":"tt14452776"`)
	byIMDB := e.movie("Filme", 2020, `"IMDB":"tt1"`)
	wrongKind := e.series("Outra", `"TMDB":"movie/5"`)
	e.fake.routes["/tv/136315"] = `{"genres":[{"id":18,"name":"Drama"}]}`
	e.fake.routes["/find/tt1"] = `{"movie_results":[{"id":3,"title":"Filme"}],"tv_results":[]}`
	e.fake.routes["/movie/3"] = `{"genres":[{"id":35,"name":"Comédia"}]}`
	e.fake.routes["/search/tv?Outra"] = `{"results":[]}`
	e.turnOn()
	if err := e.s.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := e.genres(bear); got != "Drama" {
		t.Errorf("by TMDB tag = %q", got)
	}
	if got := e.genres(byIMDB); got != "Comédia" {
		t.Errorf("by IMDb tag = %q", got)
	}
	if got := e.str(`SELECT tmdb_id FROM title_meta WHERE owner_id = ?`, bear); got != "136315" {
		t.Errorf("tmdb id = %q, want the tag's", got)
	}
	if got := e.status(wrongKind); got != "review" {
		t.Errorf("a tag of another kind must not match: status = %q", got)
	}
}

func TestUnsureGoesToReview(t *testing.T) {
	e := newEnv(t)
	twin := e.series("Origem", ``)    // two with this very title
	other := e.movie("Xyz", 2020, ``) // only other titles
	skip := e.movie("Abc", 0, ``)     // no result at all
	wrongYear := e.movie("Duna", 1950, ``)
	e.fake.routes["/search/tv?Origem"] = `{"results":[
		{"id":1,"name":"Origem","first_air_date":"2022-02-20"},{"id":2,"name":"Origem","first_air_date":"2018-01-01"}]}`
	e.fake.routes["/search/movie?Xyz"] = `{"results":[{"id":3,"title":"Xyz: O Retorno","release_date":"2020-01-01"}]}`
	e.fake.routes["/search/movie?Abc"] = `{"results":[]}`
	e.fake.routes["/search/movie?Duna"] = `{"results":[{"id":4,"title":"Duna","release_date":"2021-09-15"}]}`
	e.fake.routes["/tv/1"] = `{"genres":[{"id":9648,"name":"Mistério"}]}`
	e.turnOn()
	if err := e.s.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]string{"twin": twin, "other": other, "none": skip, "year": wrongYear} {
		if got := e.status(id); got != "review" {
			t.Errorf("%s: status = %q", name, got)
		}
		if got := e.genres(id); got != "" {
			t.Errorf("%s: got genres without certainty: %q", name, got)
		}
	}
	items, err := e.s.Review(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("review = %+v", items)
	}
	i := slices.IndexFunc(items, func(it ReviewItem) bool { return it.ID == twin })
	if i < 0 || len(items[i].Candidates) != 2 || items[i].Candidates[0].Year != 2022 {
		t.Fatalf("candidates = %+v", items)
	}

	// The user's answers.
	if err := e.s.Choose(t.Context(), twin, 1); err != nil {
		t.Fatal(err)
	}
	if e.status(twin) != "matched" || e.genres(twin) != "Mistério" {
		t.Errorf("chosen: %q %q", e.status(twin), e.genres(twin))
	}
	if err := e.s.Choose(t.Context(), other, 12345); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown code = %v", err)
	}
	if err := e.s.Skip(t.Context(), skip); err != nil {
		t.Fatal(err)
	}
	if got := e.status(skip); got != "skipped" {
		t.Errorf("skipped = %q", got)
	}
	// Answers stay, and a skipped title is not asked again.
	before := e.fake.count()
	if err := e.s.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	if e.fake.count() != before {
		t.Errorf("sync asked again: %v", e.fake.calls[before:])
	}
	if items, _ := e.s.Review(t.Context()); len(items) != 2 {
		t.Errorf("left in review = %+v", items)
	}
}

func TestBadKeyStopsTheRun(t *testing.T) {
	e := newEnv(t)
	first := e.movie("A", 2020, ``)
	e.movie("B", 2020, ``)
	e.fake.status = http.StatusUnauthorized
	e.turnOn()
	err := e.s.Sync(t.Context())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v", err)
	}
	if n := e.fake.count(); n != 1 {
		t.Errorf("kept calling after a 401: %d calls", n)
	}
	if got := e.status(first); got != "" {
		t.Errorf("wrote %q after a 401", got)
	}
	st, err := e.s.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError != CodeInvalidKey || !st.Enabled || !st.HasKey || st.Running {
		t.Errorf("status = %+v", st)
	}
	// Another key clears the error, and the run works.
	if err := e.s.SetKey(t.Context(), "another"); err != nil {
		t.Fatal(err)
	}
	if st, _ := e.s.Status(t.Context()); st.LastError != "" {
		t.Errorf("error kept after a new key: %+v", st)
	}
}

func TestNetworkFailureKeepsNothingAndHidesTheKey(t *testing.T) {
	e := newEnv(t)
	id := e.movie("A", 2020, ``)
	e.turnOn()
	e.srv.Close() // the network went away
	err := e.s.Sync(t.Context())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), testKey) {
		t.Errorf("the key is in the error: %v", err)
	}
	if got := e.status(id); got != "" {
		t.Errorf("wrote %q without an answer", got)
	}
	st, _ := e.s.Status(t.Context())
	if st.LastError != CodeUnavailable {
		t.Errorf("status = %+v", st)
	}
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), testKey) {
		t.Errorf("the key is in the status: %s", data)
	}
}

func TestKeyGoesInTheRightPlace(t *testing.T) {
	var header, query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header, query = r.Header.Get("Authorization"), r.URL.Query().Get("api_key")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()
	c := client{base: srv.URL, key: "v3key", http: srv.Client()}
	if _, err := c.search(t.Context(), kindMovie, "x"); err != nil {
		t.Fatal(err)
	}
	if header != "" || query != "v3key" {
		t.Errorf("v3 key: header %q, query %q", header, query)
	}
	c.key = "eyJ.token.v4"
	if _, err := c.search(t.Context(), kindMovie, "x"); err != nil {
		t.Fatal(err)
	}
	if header != "Bearer eyJ.token.v4" || query != "" {
		t.Errorf("v4 token: header %q, query %q", header, query)
	}
}

func TestSetKeyRefusesGarbage(t *testing.T) {
	e := newEnv(t)
	for _, k := range []string{"two words", "acentuação", strings.Repeat("x", maxKeyLen+1)} {
		if err := e.s.SetKey(t.Context(), k); !errors.Is(err, ErrInvalid) {
			t.Errorf("SetKey(%.20q) = %v", k, err)
		}
	}
	if err := e.s.SetKey(t.Context(), "  good  "); err != nil {
		t.Fatal(err)
	}
	if got := e.str(`SELECT value FROM app_state WHERE key = ?`, keyToken); got != "good" {
		t.Errorf("stored %q", got)
	}
}

func TestCleanup(t *testing.T) {
	e := newEnv(t)
	gone := e.movie("Some", 2020, ``)
	stay := e.movie("Fica", 2020, ``)
	e.fake.routes["/search/movie?Some"] = `{"results":[{"id":1,"title":"Some","release_date":"2020-01-01"}]}`
	e.fake.routes["/search/movie?Fica"] = `{"results":[{"id":2,"title":"Fica","release_date":"2020-01-01"}]}`
	e.fake.routes["/movie/1"] = `{"genres":[{"id":1,"name":"Só deste"}],"belongs_to_collection":{"id":1,"name":"Só deste"}}`
	e.fake.routes["/movie/2"] = `{"genres":[{"id":2,"name":"Outro"}]}`
	e.turnOn()
	ctx := t.Context()
	if err := e.s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	e.exec(`DELETE FROM movies WHERE id = ?`, gone)
	if err := e.s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	for table, want := range map[string]string{"title_meta": "1", "title_genres": "1", "genres": "1", "collections": "0", "collection_movies": "0"} {
		if got := e.str(`SELECT count(*) FROM ` + table); got != want {
			t.Errorf("%s has %s rows, want %s", table, got, want)
		}
	}
	if e.status(stay) != "matched" {
		t.Errorf("the other title lost its data")
	}
}

func TestRunStopsWithTheContext(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		e.s.Run(ctx)
		close(done)
	}()
	e.s.Trigger()
	cancel()
	<-done
}
