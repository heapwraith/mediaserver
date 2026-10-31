package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mediaserver/migrations"
)

// newDB creates a migrated database at file with one row to recognize it by.
func newDB(t *testing.T, file, name string) int {
	t.Helper()
	conn, err := Open(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := Migrate(t.Context(), conn, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(t.Context(),
		`INSERT INTO profiles (id, name, color, created_at) VALUES ('01a1f000-0000-7000-8000-000000000001', ?, '#4c8dff', 1)`,
		name); err != nil {
		t.Fatal(err)
	}
	v, err := CurrentVersion(t.Context(), conn)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func profileName(t *testing.T, file string) string {
	t.Helper()
	conn, err := Open(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var name string
	if err := conn.QueryRowContext(t.Context(), `SELECT name FROM profiles`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestBackup(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "mediaserver.db")
	newDB(t, file, "Sala")
	conn, err := Open(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	out, err := Backup(t.Context(), conn, dir)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if filepath.Dir(out) != dir {
		t.Errorf("backup at %s, want inside %s", out, dir)
	}
	if got := profileName(t, out); got != "Sala" {
		t.Errorf("backup profile = %q", got)
	}
}

func TestRestore(t *testing.T) {
	now := time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC)

	t.Run("replaces the database and keeps the old one", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "mediaserver.db")
		latest := newDB(t, file, "Atual")
		backup := filepath.Join(t.TempDir(), "cópia de segurança.db")
		newDB(t, backup, "Backup")

		moved, err := Restore(t.Context(), backup, file, latest, now)
		if err != nil {
			t.Fatalf("Restore: %v", err)
		}
		if want := file + ".antes-20261101-093000"; moved != want {
			t.Errorf("old database moved to %s, want %s", moved, want)
		}
		if got := profileName(t, file); got != "Backup" {
			t.Errorf("restored profile = %q", got)
		}
		if got := profileName(t, moved); got != "Atual" {
			t.Errorf("kept profile = %q", got)
		}
		if got := profileName(t, backup); got != "Backup" {
			t.Errorf("the backup file itself changed: %q", got)
		}
	})

	t.Run("first install: no database yet", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "mediaserver.db")
		backup := filepath.Join(t.TempDir(), "b.db")
		latest := newDB(t, backup, "Backup")
		moved, err := Restore(t.Context(), backup, file, latest, now)
		if err != nil || moved != "" {
			t.Fatalf("Restore = %q, %v", moved, err)
		}
		if got := profileName(t, file); got != "Backup" {
			t.Errorf("restored profile = %q", got)
		}
	})

	for _, c := range []struct {
		name string
		make func(t *testing.T, path string) int // returns the version the binary knows
		want error
	}{
		{"not a database", func(t *testing.T, path string) int {
			if err := os.WriteFile(path, []byte("not sqlite at all"), 0o600); err != nil {
				t.Fatal(err)
			}
			return 9
		}, ErrBadBackup},
		{"sqlite without our tables", func(t *testing.T, path string) int {
			conn, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if _, err := conn.ExecContext(context.Background(), `CREATE TABLE x (a)`); err != nil {
				t.Fatal(err)
			}
			return 9
		}, ErrBadBackup},
		{"made by a newer server", func(t *testing.T, path string) int {
			return newDB(t, path, "Futuro") - 1
		}, ErrNewerBackup},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "mediaserver.db")
			newDB(t, file, "Atual")
			backup := filepath.Join(t.TempDir(), "b.db")
			known := c.make(t, backup)
			if _, err := Restore(t.Context(), backup, file, known, now); !errors.Is(err, c.want) {
				t.Fatalf("Restore err = %v, want %v", err, c.want)
			}
			if got := profileName(t, file); got != "Atual" {
				t.Errorf("a refused restore changed the database: %q", got)
			}
			if leftovers, _ := filepath.Glob(filepath.Join(dir, "*.restore*")); len(leftovers) > 0 {
				t.Errorf("leftovers: %v", leftovers)
			}
		})
	}
}
