// Package db opens the SQLite database and applies the embedded migrations.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure Go SQLite driver (no CGO)
)

// FileName is the database file inside DATA_DIR.
const FileName = "mediaserver.db"

// pragmas are applied to every new connection by the driver.
var pragmas = []string{
	"journal_mode(WAL)",
	"busy_timeout(5000)",
	"foreign_keys(1)",
	"synchronous(NORMAL)",
}

// Open opens (creating if needed) the SQLite database at file in WAL mode
// and checks the connection.
func Open(ctx context.Context, file string) (*sql.DB, error) {
	params := make([]string, 0, len(pragmas)+1)
	for _, p := range pragmas {
		params = append(params, "_pragma="+p)
	}
	params = append(params, "_txlock=immediate")

	conn, err := sql.Open("sqlite", file+"?"+strings.Join(params, "&"))
	if err != nil {
		return nil, fmt.Errorf("open db %s: %w", file, err)
	}
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping db %s: %w", file, err)
	}
	return conn, nil
}

// Migration is one numbered SQL file.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// LoadMigrations reads every NNN_name.sql file in the root of fsys, sorted by
// version.
func LoadMigrations(fsys fs.FS) ([]Migration, error) {
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	migs := make([]Migration, 0, len(files))
	seen := make(map[int]string, len(files))
	for _, name := range files {
		prefix, _, ok := strings.Cut(name, "_")
		version, err := strconv.Atoi(prefix)
		if !ok || err != nil || version <= 0 {
			return nil, fmt.Errorf("migration %s: name must look like 001_name.sql", name)
		}
		if other, dup := seen[version]; dup {
			return nil, fmt.Errorf("migrations %s and %s share version %d", other, name, version)
		}
		seen[version] = name
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		migs = append(migs, Migration{Version: version, Name: path.Base(name), SQL: string(body)})
	}
	slices.SortFunc(migs, func(a, b Migration) int { return a.Version - b.Version })
	return migs, nil
}

// Migrate applies the migrations of fsys that are not yet recorded in
// schema_migrations, each one in its own transaction. It returns the versions
// applied in this call.
func Migrate(ctx context.Context, conn *sql.DB, fsys fs.FS) ([]int, error) {
	migs, err := LoadMigrations(fsys)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT    NOT NULL,
		applied_at INTEGER NOT NULL
	) STRICT`); err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}

	current, err := CurrentVersion(ctx, conn)
	if err != nil {
		return nil, err
	}
	var applied []int
	for _, m := range migs {
		if m.Version <= current {
			continue
		}
		if err := apply(ctx, conn, m); err != nil {
			return applied, err
		}
		applied = append(applied, m.Version)
	}
	return applied, nil
}

// CurrentVersion returns the highest applied migration version (0 if none).
func CurrentVersion(ctx context.Context, conn *sql.DB) (int, error) {
	var v sql.NullInt64
	err := conn.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return int(v.Int64), nil
}

func apply(ctx context.Context, conn *sql.DB, m Migration) (err error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migration %s: begin: %w", m.Name, err)
	}
	defer func() {
		if err == nil {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			err = errors.Join(err, rbErr)
		}
	}()
	if _, err = tx.ExecContext(ctx, m.SQL); err != nil {
		return fmt.Errorf("migration %s: %w", m.Name, err)
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.Version, m.Name, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("migration %s: record: %w", m.Name, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("migration %s: commit: %w", m.Name, err)
	}
	return nil
}
