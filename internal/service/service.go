// Package service installs the server as a system service that starts with
// the computer: systemd (Linux), launchd (macOS) or the Windows Service
// Manager. `mediaserver service install|uninstall|start|stop|status`.
package service

import (
	"encoding/xml"
	"errors"
	"strings"
)

const (
	// Name of the systemd unit and of the Windows service.
	Name = "mediaserver"
	// Label of the launchd agent.
	Label = "com.mediaserver"
	// Description shown by the service managers.
	Description = "mediaserver: servidor de mídia local"
)

// ErrUnsupported means this operating system has no service manager we know.
var ErrUnsupported = errors.New("services are supported on Linux (systemd), macOS (launchd) and Windows")

// ErrNotInstalled means there is no service to act on.
var ErrNotInstalled = errors.New("the service is not installed (mediaserver service install)")

// Spec is how the service runs the server.
type Spec struct {
	// Exe is the absolute path of the mediaserver binary.
	Exe string
	// Args are the server flags (the ones given to `service install`).
	Args []string
	// WorkDir is where relative paths (./data, config.toml) are resolved.
	WorkDir string
	// User runs a systemd system unit ("" = root). Ignored elsewhere.
	User string
	// LogFile receives the output under launchd.
	LogFile string
}

// Manager installs and controls the service on this computer.
type Manager interface {
	Install(s Spec) (where string, err error)
	Uninstall() error
	Start() error
	Stop() error
	// Status is a short state: running, stopped, not installed…
	Status() (string, error)
}

// SystemdUnit is the unit file of s. user: a user unit (systemctl --user).
func SystemdUnit(s Spec, user bool) string {
	var b strings.Builder
	b.WriteString("[Unit]\nDescription=" + Description + "\n")
	b.WriteString("After=network-online.target\nWants=network-online.target\n\n")
	b.WriteString("[Service]\nType=simple\n")
	b.WriteString("ExecStart=" + systemdQuote(s.Exe))
	for _, a := range s.Args {
		b.WriteString(" " + systemdQuote(a))
	}
	b.WriteString("\n")
	if s.WorkDir != "" {
		b.WriteString("WorkingDirectory=" + systemdEscape(s.WorkDir) + "\n")
	}
	if s.User != "" && !user {
		b.WriteString("User=" + s.User + "\n")
	}
	// SIGTERM is the graceful shutdown (FFmpeg processes stopped, database closed).
	b.WriteString("Restart=on-failure\nRestartSec=5\nTimeoutStopSec=30\n\n")
	b.WriteString("[Install]\n")
	if user {
		b.WriteString("WantedBy=default.target\n")
	} else {
		b.WriteString("WantedBy=multi-user.target\n")
	}
	return b.String()
}

// systemdEscape escapes the specifier character (%).
func systemdEscape(s string) string {
	return strings.ReplaceAll(s, "%", "%%")
}

// systemdQuote quotes one ExecStart word (spaces, quotes, $ and % kept literal).
func systemdQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%", "$", "$$")
	return `"` + r.Replace(s) + `"`
}

// LaunchdPlist is the launchd agent of s: starts at login, restarts after a
// crash (not after a clean stop).
func LaunchdPlist(s Spec) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + Label + `</string>
	<key>ProgramArguments</key>
	<array>
`)
	for _, a := range append([]string{s.Exe}, s.Args...) {
		b.WriteString("\t\t<string>" + xmlEscape(a) + "</string>\n")
	}
	b.WriteString("\t</array>\n")
	if s.WorkDir != "" {
		b.WriteString("\t<key>WorkingDirectory</key>\n\t<string>" + xmlEscape(s.WorkDir) + "</string>\n")
	}
	b.WriteString(`	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
`)
	if s.LogFile != "" {
		for _, k := range []string{"StandardOutPath", "StandardErrorPath"} {
			b.WriteString("\t<key>" + k + "</key>\n\t<string>" + xmlEscape(s.LogFile) + "</string>\n")
		}
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s)) // writing to a strings.Builder never fails
	return b.String()
}
