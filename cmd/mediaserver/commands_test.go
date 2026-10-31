package main

import (
	"bytes"
	"context"
	"database/sql"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"mediaserver/internal/db"
	"mediaserver/migrations"
)

func migratedDB(t *testing.T, file, profile string) {
	t.Helper()
	conn, err := db.Open(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := db.Migrate(t.Context(), conn, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(t.Context(),
		`INSERT INTO profiles (id, name, color, created_at) VALUES ('01a1f000-0000-7000-8000-000000000002', ?, '#4c8dff', 1)`,
		profile); err != nil {
		t.Fatal(err)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestRestoreCmd(t *testing.T) {
	media, data := t.TempDir(), t.TempDir()
	migratedDB(t, filepath.Join(data, db.FileName), "Atual")
	backup := filepath.Join(t.TempDir(), "mediaserver-20261101-0930.db")
	migratedDB(t, backup, "Backup")
	flags := func(port int) []string {
		return []string{backup, "-media-dir", media, "-data-dir", data, "-port", strconv.Itoa(port)}
	}

	// A server answering on the port: refused.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"name":"mediaserver","status":"ok"}`))
	}))
	defer srv.Close()
	busy := srv.Listener.Addr().(*net.TCPAddr).Port
	var out, errOut bytes.Buffer
	if code := restoreCmd(flags(busy), &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "stop it first") {
		t.Fatalf("restore with the server up = %d %q", code, errOut.String())
	}

	out.Reset()
	if code := restoreCmd(flags(freePort(t)), &out, &errOut); code != 0 {
		t.Fatalf("restore = %d %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "previous database kept as") {
		t.Errorf("output = %q", out.String())
	}
	conn, err := sql.Open("sqlite", filepath.Join(data, db.FileName))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var name string
	if err := conn.QueryRowContext(context.Background(), `SELECT name FROM profiles`).Scan(&name); err != nil || name != "Backup" {
		t.Fatalf("restored profile = %q, %v", name, err)
	}

	if code := restoreCmd(nil, &out, &errOut); code != 2 {
		t.Errorf("restore without a file = %d", code)
	}
}

func TestInstallSpec(t *testing.T) {
	media := t.TempDir()
	env := map[string]string{"MEDIA_DIR": media, "HWACCEL": "none"}
	spec, cfg, err := installSpec([]string{"-port", "8096"}, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-port", "8096", "-media-dir=" + media, "-hwaccel=none"}
	if !slices.Equal(spec.Args, want) {
		t.Errorf("args = %q, want %q", spec.Args, want)
	}
	wd, _ := os.Getwd()
	if spec.WorkDir != wd || !filepath.IsAbs(spec.Exe) || cfg.Port != 8096 {
		t.Errorf("spec = %+v, port %d", spec, cfg.Port)
	}
	// A configuration the server would refuse is refused now, not at boot.
	if _, _, err := installSpec([]string{"-port", "0"}, func(k string) string { return env[k] }); err == nil {
		t.Error("invalid port accepted")
	}
}

func TestServiceUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := serviceCmd([]string{"bogus"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "install [flags]") {
		t.Errorf("service bogus = %d %q", code, errOut.String())
	}
	if _, ok := command([]string{"-port", "1"}, &out, &errOut); ok {
		t.Error("flags taken for a command")
	}
}
