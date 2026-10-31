package db

import (
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"mediaserver/migrations"
)

func openTemp(t *testing.T) *sql.DB {
	t.Helper()
	// Space, accent and Unicode in the path on purpose.
	file := filepath.Join(t.TempDir(), "bibliotéca ライブラリ.db")
	conn, err := Open(t.Context(), file)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestOpenPragmas(t *testing.T) {
	conn := openTemp(t)
	tests := []struct {
		pragma string
		want   string
	}{
		{"journal_mode", "wal"},
		{"busy_timeout", "5000"},
		{"foreign_keys", "1"},
		{"synchronous", "1"}, // NORMAL
	}
	for _, tt := range tests {
		t.Run(tt.pragma, func(t *testing.T) {
			var got string
			if err := conn.QueryRowContext(t.Context(), "PRAGMA "+tt.pragma).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("PRAGMA %s = %q, want %q", tt.pragma, got, tt.want)
			}
		})
	}
}

func TestMigrateEmbedded(t *testing.T) {
	conn := openTemp(t)
	ctx := t.Context()

	applied, err := Migrate(ctx, conn, migrations.FS)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !slices.Contains(applied, 1) {
		t.Fatalf("applied = %v, want it to contain 1", applied)
	}

	for _, table := range []string{
		"libraries", "media_files", "movies", "series", "seasons",
		"episodes", "subtitles", "profiles", "progress",
	} {
		var n int
		err := conn.QueryRowContext(ctx,
			`SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = ?`, table).Scan(&n)
		if err != nil || n != 1 {
			t.Errorf("table %s: count=%d err=%v", table, n, err)
		}
	}

	// Idempotent: a second run applies nothing.
	again, err := Migrate(ctx, conn, migrations.FS)
	if err != nil || len(again) != 0 {
		t.Fatalf("second Migrate: applied=%v err=%v", again, err)
	}

	// Foreign keys are enforced.
	_, err = conn.ExecContext(ctx,
		`INSERT INTO seasons (id, series_id, number) VALUES ('s1', 'missing', 1)`)
	if err == nil {
		t.Fatal("insert with dangling foreign key succeeded, want error")
	}
}

func TestMigrateOrderAndRollback(t *testing.T) {
	conn := openTemp(t)
	ctx := t.Context()
	fsys := fstest.MapFS{
		"002_b.sql": {Data: []byte(`CREATE TABLE b (id INTEGER);`)},
		"001_a.sql": {Data: []byte(`CREATE TABLE a (id INTEGER);`)},
		"003_c.sql": {Data: []byte(`CREATE TABLE c (id INTEGER); THIS IS NOT SQL;`)},
	}
	applied, err := Migrate(ctx, conn, fsys)
	if err == nil || !strings.Contains(err.Error(), "003_c.sql") {
		t.Fatalf("got err %v, want failure in 003_c.sql", err)
	}
	if !slices.Equal(applied, []int{1, 2}) {
		t.Fatalf("applied = %v, want [1 2]", applied)
	}
	v, err := CurrentVersion(ctx, conn)
	if err != nil || v != 2 {
		t.Fatalf("CurrentVersion = %d, %v; want 2", v, err)
	}
	var n int
	_ = conn.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE name = 'c'`).Scan(&n)
	if n != 0 {
		t.Fatal("table c exists: failed migration was not rolled back")
	}
}

func TestLoadMigrationsBadNames(t *testing.T) {
	tests := []struct {
		name string
		fsys fstest.MapFS
	}{
		{"no number", fstest.MapFS{"init.sql": {}}},
		{"zero", fstest.MapFS{"000_init.sql": {}}},
		{"duplicate", fstest.MapFS{"001_a.sql": {}, "1_b.sql": {}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := LoadMigrations(tt.fsys); err == nil {
				t.Fatal("want error")
			}
		})
	}
}
