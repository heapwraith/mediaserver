package main

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"mediaserver/internal/config"
	"mediaserver/internal/db"
	"mediaserver/internal/service"
	"mediaserver/migrations"
)

// command runs `mediaserver <command> ...` and reports whether args name
// one; without a command the binary is the server.
func command(args []string, stdout, stderr io.Writer) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	switch args[0] {
	case "restore":
		return restoreCmd(args[1:], stdout, stderr), true
	case "service":
		return serviceCmd(args[1:], stdout, stderr), true
	}
	return 0, false
}

const restoreUsage = `usage: mediaserver restore <backup.db> [server flags]

Replaces the database in DATA_DIR with a backup downloaded from
Configurações → Baixar backup. Stop the server first. The current database
is kept next to it as mediaserver.db.antes-<date>.
`

// restoreCmd: `mediaserver restore <file> [flags]`, with the server stopped.
func restoreCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fmt.Fprint(stderr, restoreUsage)
		return 2
	}
	file := args[0]
	cfg, _, err := config.Load(args[1:], os.Getenv, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "mediaserver: config:", err)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if serverRunning(ctx, cfg) {
		fmt.Fprintf(stderr, "mediaserver: the server is running on port %d: stop it first (mediaserver service stop)\n", cfg.Port)
		return 1
	}
	migs, err := db.LoadMigrations(migrations.FS)
	if err != nil || len(migs) == 0 {
		fmt.Fprintln(stderr, "mediaserver: migrations:", err)
		return 1
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		fmt.Fprintln(stderr, "mediaserver:", err)
		return 1
	}
	target := filepath.Join(cfg.DataDir, db.FileName)
	moved, err := db.Restore(ctx, file, target, migs[len(migs)-1].Version, time.Now())
	if err != nil {
		fmt.Fprintln(stderr, "mediaserver: restore:", err)
		return 1
	}
	fmt.Fprintln(stdout, "restored", file, "to", target)
	if moved != "" {
		fmt.Fprintln(stdout, "previous database kept as", moved)
	}
	return 0
}

// serverRunning reports whether a mediaserver answers on the configured port.
func serverRunning(ctx context.Context, cfg config.Config) bool {
	host := cfg.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	url := "http://" + net.JoinHostPort(host, strconv.Itoa(cfg.Port)) + "/api/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var body struct {
		Name string `json:"name"`
	}
	return json.UnmarshalRead(res.Body, &body) == nil && body.Name == "mediaserver"
}

const serviceUsage = `usage: mediaserver service <command> [server flags]

  install [flags]  start with the computer, with these flags (and the
                   config.toml / environment variables set now)
  uninstall        remove the service
  start | stop     start or stop it now
  status           running, stopped, not installed…

Linux: systemd (with sudo: system service; without: user service).
macOS: launchd agent of the user. Windows: run from an administrator terminal.
`

// serviceCmd: `mediaserver service <command> [flags]`.
func serviceCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, serviceUsage)
		return 2
	}
	m := service.New()
	var err error
	switch args[0] {
	case "install":
		return serviceInstall(m, args[1:], stdout, stderr)
	case "uninstall":
		if err = m.Uninstall(); err == nil {
			fmt.Fprintln(stdout, "service removed")
		}
	case "start":
		if err = m.Start(); err == nil {
			fmt.Fprintln(stdout, "service started")
		}
	case "stop":
		if err = m.Stop(); err == nil {
			fmt.Fprintln(stdout, "service stopped")
		}
	case "status":
		var state string
		if state, err = m.Status(); err == nil {
			fmt.Fprintln(stdout, state)
		}
	case "run": // started by the Windows service manager (see internal/service)
		return serviceRun(args[1:], stderr)
	default:
		fmt.Fprint(stderr, serviceUsage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "mediaserver: service:", err)
		return 1
	}
	return 0
}

// serviceInstall checks the configuration as the service will see it and
// installs the service. Relative paths keep working: the service starts in
// the current folder.
func serviceInstall(m service.Manager, args []string, stdout, stderr io.Writer) int {
	spec, cfg, err := installSpec(args, os.Getenv)
	if err != nil {
		fmt.Fprintln(stderr, "mediaserver: service:", err)
		return 2
	}
	where, err := m.Install(spec)
	if err != nil {
		fmt.Fprintln(stderr, "mediaserver: service:", err)
		return 1
	}
	fmt.Fprintln(stdout, "service installed:", where)
	fmt.Fprintf(stdout, "open http://localhost:%d (status: mediaserver service status)\n", cfg.Port)
	return 0
}

// installSpec builds what the service runs: this binary, the flags given
// and the environment settings turned into flags (a service does not get the
// shell's environment).
func installSpec(args []string, getenv func(string) string) (service.Spec, config.Config, error) {
	cfg, _, err := config.Load(args, getenv, io.Discard)
	if err != nil {
		return service.Spec{}, cfg, err
	}
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return service.Spec{}, cfg, fmt.Errorf("find the binary: %w", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		return service.Spec{}, cfg, err
	}
	flags := append(slices.Clone(args), config.EnvAsFlags(args, getenv)...)
	return service.Spec{Exe: exe, Args: flags, WorkDir: wd}, cfg, nil
}

// serviceRun runs the server under the Windows service manager:
// `mediaserver service run <folder> [flags]`.
func serviceRun(args []string, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "mediaserver: service run: missing folder")
		return 2
	}
	if err := os.Chdir(args[0]); err != nil {
		fmt.Fprintln(stderr, "mediaserver: service run:", err)
		return 1
	}
	start := time.Now()
	code := 0
	err := service.Run(func(ctx context.Context) int {
		code = serveMain(ctx, args[1:], start, true)
		return code
	})
	if err != nil {
		fmt.Fprintln(stderr, "mediaserver: service run:", err)
		return 1
	}
	return code
}
