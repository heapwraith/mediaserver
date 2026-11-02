// SPDX-License-Identifier: GPL-3.0-or-later

package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

var (
	// ErrBadBackup means the file is not a mediaserver database (or is damaged).
	ErrBadBackup = errors.New("not a valid mediaserver backup")
	// ErrNewerBackup means the backup comes from a newer version of the server.
	ErrNewerBackup = errors.New("backup made by a newer mediaserver")
)

// Backup writes a consistent copy of the database to a new file in dir and
// returns its path. VACUUM INTO works while the server keeps using the
// database, and the copy has no WAL to carry along.
//
// The copy has no secrets: the session secret, the TMDB key and the PIN
// hashes are removed (a 4-digit PIN hash is broken offline in minutes, and
// the secret would let anyone forge a session). After a restore the sessions
// are signed with a new secret and the profiles come back without PIN.
func Backup(ctx context.Context, conn *sql.DB, dir string) (string, error) {
	tmp, err := tempName(dir, "backup-raw-*.db")
	if err != nil {
		return "", err
	}
	defer removeDB(tmp)
	if _, err := conn.ExecContext(ctx, `VACUUM INTO ?`, tmp); err != nil {
		return "", fmt.Errorf("backup %s: %w", tmp, err)
	}
	if err := stripSecrets(ctx, tmp); err != nil {
		return "", err
	}
	// A second VACUUM INTO drops the free pages where the removed values
	// would still be readable.
	name, err := tempName(dir, "backup-*.db")
	if err != nil {
		return "", err
	}
	src, err := Open(ctx, tmp)
	if err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	defer src.Close()
	if _, err := src.ExecContext(ctx, `VACUUM INTO ?`, name); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("backup %s: %w", name, err)
	}
	return name, nil
}

// secretKeys are the app_state keys that never go into a backup.
var secretKeys = []string{"session_secret", "tmdb_key"}

func stripSecrets(ctx context.Context, file string) (err error) {
	c, err := Open(ctx, file)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	defer func() {
		if cerr := c.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`DELETE FROM app_state WHERE key IN (?, ?)`, []any{secretKeys[0], secretKeys[1]}},
		{`UPDATE profiles SET pin_hash = NULL`, nil},
	} {
		if _, err := c.ExecContext(ctx, q.sql, q.args...); err != nil {
			return fmt.Errorf("backup: remove secrets: %w", err)
		}
	}
	return nil
}

// tempName reserves a unique file name in dir and leaves no file behind.
func tempName(dir, pattern string) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	name := f.Name()
	if err := errors.Join(f.Close(), os.Remove(name)); err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	return name, nil
}

// Restore puts the backup src in place of dbFile; the server must be
// stopped. The backup is copied next to dbFile and checked first (the
// original files are not touched until then): it must be an intact
// mediaserver database whose schema this binary knows (maxVersion; older
// ones are migrated on the next start). The current database is kept as
// "<dbFile>.antes-AAAAMMDD-HHMMSS", whose path is returned ("" when there
// was none).
func Restore(ctx context.Context, src, dbFile string, maxVersion int, now time.Time) (string, error) {
	tmp := dbFile + ".restore"
	if err := copyFile(src, tmp); err != nil {
		return "", fmt.Errorf("restore: %w", err)
	}
	if err := check(ctx, tmp, maxVersion); err != nil {
		removeDB(tmp)
		return "", err
	}

	moved := ""
	if _, err := os.Stat(dbFile); err == nil {
		moved = dbFile + ".antes-" + now.Format("20060102-150405")
		for _, suffix := range []string{"", "-wal", "-shm"} {
			err := os.Rename(dbFile+suffix, moved+suffix)
			if err != nil && (suffix == "" || !errors.Is(err, os.ErrNotExist)) {
				removeDB(tmp)
				return "", fmt.Errorf("restore: keep current database: %w", err)
			}
		}
	}
	if err := os.Rename(tmp, dbFile); err != nil {
		return moved, fmt.Errorf("restore: %w (the previous database is in %s)", err, moved)
	}
	return moved, nil
}

// check opens a candidate database and validates it.
func check(ctx context.Context, file string, maxVersion int) (err error) {
	conn, err := Open(ctx, file)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrBadBackup, err)
	}
	defer func() {
		if cerr := conn.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()
	var integrity string
	if err := conn.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return fmt.Errorf("%w: %w", ErrBadBackup, err)
	}
	if integrity != "ok" {
		return fmt.Errorf("%w: integrity check: %s", ErrBadBackup, integrity)
	}
	version, err := CurrentVersion(ctx, conn)
	if err != nil || version == 0 {
		return fmt.Errorf("%w: no schema version", ErrBadBackup)
	}
	if version > maxVersion {
		return fmt.Errorf("%w: schema %d, this server knows up to %d", ErrNewerBackup, version, maxVersion)
	}
	return nil
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(filepath.Clean(src))
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(filepath.Clean(dst), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(out, in)
	return err
}

// removeDB deletes a database file with its WAL and shared memory.
func removeDB(file string) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(file + suffix)
	}
}
