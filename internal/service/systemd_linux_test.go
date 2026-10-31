//go:build linux

package service

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func fakeSystemd(t *testing.T, root bool) (*systemd, *[]string) {
	t.Helper()
	var calls []string
	m := &systemd{
		systemDir: filepath.Join(t.TempDir(), "system"),
		userDir:   filepath.Join(t.TempDir(), "user"),
		root:      root,
		sudoUser:  "ana",
		systemctl: func(args ...string) (string, error) {
			calls = append(calls, strings.Join(args, " "))
			if slices.Contains(args, "is-active") {
				return "active\n", nil
			}
			return "", nil
		},
	}
	return m, &calls
}

func TestSystemdUser(t *testing.T) {
	m, calls := fakeSystemd(t, false)
	if s, _ := m.Status(); s != "not installed" {
		t.Errorf("status before install = %q", s)
	}
	where, err := m.Install(Spec{Exe: "/bin/mediaserver"})
	if err != nil {
		t.Fatal(err)
	}
	path, hint, _ := strings.Cut(where, "\n")
	if !strings.Contains(hint, "enable-linger") {
		t.Errorf("no linger hint: %q", where)
	}
	unit, _ := os.ReadFile(path)
	if !strings.Contains(string(unit), "WantedBy=default.target") || strings.Contains(string(unit), "User=") {
		t.Errorf("user unit:\n%s", unit)
	}
	if s, _ := m.Status(); s != "running" {
		t.Errorf("status = %q", s)
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("unit still there: %v", err)
	}
	want := []string{
		"--user daemon-reload", "--user enable --now mediaserver.service",
		"--user is-active mediaserver.service", "--user stop mediaserver.service",
		"--user disable --now mediaserver.service", "--user daemon-reload",
	}
	if !slices.Equal(*calls, want) {
		t.Errorf("systemctl calls = %q, want %q", *calls, want)
	}
	if err := m.Start(); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("start after uninstall = %v", err)
	}
}

func TestSystemdRoot(t *testing.T) {
	m, calls := fakeSystemd(t, true)
	path, err := m.Install(Spec{Exe: "/bin/mediaserver"})
	if err != nil {
		t.Fatal(err)
	}
	unit, _ := os.ReadFile(path)
	if !strings.Contains(string(unit), "User=ana\n") || !strings.Contains(string(unit), "multi-user.target") {
		t.Errorf("system unit:\n%s", unit)
	}
	if (*calls)[1] != "enable --now mediaserver.service" {
		t.Errorf("calls = %q", *calls)
	}
	// Without sudo, a system-wide service asks for it.
	m.root = false
	if err := m.Stop(); err == nil || !strings.Contains(err.Error(), "sudo") {
		t.Errorf("stop without sudo = %v", err)
	}
}
