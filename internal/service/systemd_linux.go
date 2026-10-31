//go:build linux

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// systemd: a system unit when run as root (sudo), else a user unit.
type systemd struct {
	systemDir string // /etc/systemd/system
	userDir   string // ~/.config/systemd/user
	root      bool
	sudoUser  string
	// systemctl runs systemctl with args and returns its output.
	systemctl func(args ...string) (string, error)
}

// New returns the manager of this computer.
func New() Manager {
	home, _ := os.UserHomeDir()
	return &systemd{
		systemDir: "/etc/systemd/system",
		userDir:   filepath.Join(home, ".config", "systemd", "user"),
		root:      os.Geteuid() == 0,
		sudoUser:  os.Getenv("SUDO_USER"),
		systemctl: func(args ...string) (string, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput() //nolint:gosec // fixed program; args are ours
			if err != nil {
				return string(out), fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
			}
			return string(out), nil
		},
	}
}

func (m *systemd) unitPath(user bool) string {
	if user {
		return filepath.Join(m.userDir, Name+".service")
	}
	return filepath.Join(m.systemDir, Name+".service")
}

// installed finds the unit: the system one first. user tells which.
func (m *systemd) installed() (user bool, err error) {
	if _, err := os.Stat(m.unitPath(false)); err == nil {
		if !m.root {
			return false, errors.New("the service is installed system-wide: run this command with sudo")
		}
		return false, nil
	}
	if _, err := os.Stat(m.unitPath(true)); err == nil {
		return true, nil
	}
	return false, ErrNotInstalled
}

func (m *systemd) ctl(user bool, args ...string) error {
	if user {
		args = append([]string{"--user"}, args...)
	}
	_, err := m.systemctl(args...)
	return err
}

func (m *systemd) Install(s Spec) (string, error) {
	user := !m.root
	if !user && s.User == "" {
		s.User = m.sudoUser // the media belong to the user who ran sudo
	}
	path := m.unitPath(user)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // systemd unit folders are world-readable
		return "", fmt.Errorf("install service: %w", err)
	}
	if err := os.WriteFile(path, []byte(SystemdUnit(s, user)), 0o644); err != nil { //nolint:gosec // unit files are world-readable
		return "", fmt.Errorf("install service: %w", err)
	}
	if err := m.ctl(user, "daemon-reload"); err != nil {
		return path, err
	}
	if err := m.ctl(user, "enable", "--now", Name+".service"); err != nil {
		return path, err
	}
	if user {
		// A user unit starts at login; linger starts it at boot.
		path += "\n(user service: it starts when you log in; to start it at boot without logging in: sudo loginctl enable-linger $USER)"
	}
	return path, nil
}

func (m *systemd) Uninstall() error {
	user, err := m.installed()
	if err != nil {
		return err
	}
	_ = m.ctl(user, "disable", "--now", Name+".service") // already stopped is fine
	if err := os.Remove(m.unitPath(user)); err != nil {
		return fmt.Errorf("uninstall service: %w", err)
	}
	return m.ctl(user, "daemon-reload")
}

func (m *systemd) Start() error { return m.action("start") }
func (m *systemd) Stop() error  { return m.action("stop") }

func (m *systemd) action(verb string) error {
	user, err := m.installed()
	if err != nil {
		return err
	}
	return m.ctl(user, verb, Name+".service")
}

func (m *systemd) Status() (string, error) {
	user, err := m.installed()
	if errors.Is(err, ErrNotInstalled) {
		return "not installed", nil
	}
	if err != nil {
		return "", err
	}
	args := []string{"is-active", Name + ".service"}
	if user {
		args = append([]string{"--user"}, args...)
	}
	// is-active exits non-zero when the unit is not running: the text says why.
	out, _ := m.systemctl(args...)
	state := strings.TrimSpace(out)
	if state == "active" {
		return "running", nil
	}
	if state == "" {
		state = "unknown"
	}
	return state, nil
}

// Run is only used by Windows services.
func Run(func(ctx context.Context) int) error { return ErrUnsupported }
