//go:build darwin

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// launchd: a LaunchAgent of the user (starts at login; the media in the
// user's folders stay readable, which a LaunchDaemon may not get).
type launchd struct {
	dir     string // ~/Library/LaunchAgents
	logDir  string // ~/Library/Logs
	domain  string // gui/<uid>
	launchc func(args ...string) (string, error)
}

// New returns the manager of this computer.
func New() Manager {
	home, _ := os.UserHomeDir()
	return &launchd{
		dir:    filepath.Join(home, "Library", "LaunchAgents"),
		logDir: filepath.Join(home, "Library", "Logs"),
		domain: "gui/" + strconv.Itoa(os.Getuid()),
		launchc: func(args ...string) (string, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, "launchctl", args...).CombinedOutput() //nolint:gosec // fixed program; args are ours
			if err != nil {
				return string(out), fmt.Errorf("launchctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
			}
			return string(out), nil
		},
	}
}

func (m *launchd) plist() string  { return filepath.Join(m.dir, Label+".plist") }
func (m *launchd) target() string { return m.domain + "/" + Label }

func (m *launchd) installed() error {
	if _, err := os.Stat(m.plist()); err != nil {
		return ErrNotInstalled
	}
	return nil
}

func (m *launchd) Install(s Spec) (string, error) {
	if s.LogFile == "" {
		s.LogFile = filepath.Join(m.logDir, Name+".log")
	}
	if err := os.MkdirAll(m.dir, 0o755); err != nil { //nolint:gosec // standard LaunchAgents folder
		return "", fmt.Errorf("install service: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.LogFile), 0o755); err != nil { //nolint:gosec // standard Logs folder
		return "", fmt.Errorf("install service: %w", err)
	}
	if err := os.WriteFile(m.plist(), []byte(LaunchdPlist(s)), 0o644); err != nil { //nolint:gosec // plists are world-readable
		return "", fmt.Errorf("install service: %w", err)
	}
	_, _ = m.launchc("bootout", m.target()) // an older version may be loaded
	_, err := m.launchc("bootstrap", m.domain, m.plist())
	return m.plist(), err
}

func (m *launchd) Uninstall() error {
	if err := m.installed(); err != nil {
		return err
	}
	_, _ = m.launchc("bootout", m.target())
	if err := os.Remove(m.plist()); err != nil {
		return fmt.Errorf("uninstall service: %w", err)
	}
	return nil
}

func (m *launchd) Start() error {
	if err := m.installed(); err != nil {
		return err
	}
	if _, err := m.launchc("kickstart", m.target()); err != nil {
		// Not loaded (stopped with bootout, or after an update): load it.
		_, err = m.launchc("bootstrap", m.domain, m.plist())
		return err
	}
	return nil
}

// Stop sends SIGTERM: a clean exit, which KeepAlive does not restart.
func (m *launchd) Stop() error {
	if err := m.installed(); err != nil {
		return err
	}
	_, err := m.launchc("kill", "SIGTERM", m.target())
	return err
}

func (m *launchd) Status() (string, error) {
	if err := m.installed(); errors.Is(err, ErrNotInstalled) {
		return "not installed", nil
	}
	out, err := m.launchc("print", m.target())
	if err != nil {
		return "not loaded", nil //nolint:nilerr // not loaded is a state, not a failure
	}
	for line := range strings.SplitSeq(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok && strings.TrimSpace(k) == "state" {
			return strings.TrimSpace(v), nil
		}
	}
	return "loaded", nil
}

// Run is only used by Windows services.
func Run(func(ctx context.Context) int) error { return ErrUnsupported }
