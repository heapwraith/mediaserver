package service

import (
	"strings"
	"testing"
)

var spec = Spec{
	Exe:     "/opt/media server/mediaserver",
	Args:    []string{"-media-dir", "/srv/Mídias & Filmes", "-config", "/etc/ms/50%.toml"},
	WorkDir: "/home/ana/media server",
	User:    "ana",
	LogFile: "/Users/ana/Library/Logs/mediaserver.log",
}

func TestSystemdUnit(t *testing.T) {
	for _, c := range []struct {
		name string
		user bool
		want []string
		not  []string
	}{
		{"system", false, []string{
			`ExecStart="/opt/media server/mediaserver" "-media-dir" "/srv/Mídias & Filmes" "-config" "/etc/ms/50%%.toml"`,
			"WorkingDirectory=/home/ana/media server\n",
			"User=ana\n",
			"Restart=on-failure\n",
			"WantedBy=multi-user.target\n",
		}, []string{"default.target"}},
		{"user", true, []string{"WantedBy=default.target\n"}, []string{"User=", "multi-user"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			unit := SystemdUnit(spec, c.user)
			for _, w := range c.want {
				if !strings.Contains(unit, w) {
					t.Errorf("unit lacks %q:\n%s", w, unit)
				}
			}
			for _, n := range c.not {
				if strings.Contains(unit, n) {
					t.Errorf("unit has %q:\n%s", n, unit)
				}
			}
		})
	}
}

func TestSystemdQuote(t *testing.T) {
	for in, want := range map[string]string{
		`plain`:     `"plain"`,
		`a "b"`:     `"a \"b\""`,
		`C:\x`:      `"C:\\x"`,
		`$HOME 10%`: `"$$HOME 10%%"`,
	} {
		if got := systemdQuote(in); got != want {
			t.Errorf("systemdQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestLaunchdPlist(t *testing.T) {
	p := LaunchdPlist(spec)
	for _, w := range []string{
		"<string>com.mediaserver</string>",
		"<string>/opt/media server/mediaserver</string>\n\t\t<string>-media-dir</string>\n\t\t<string>/srv/Mídias &amp; Filmes</string>",
		"<key>WorkingDirectory</key>\n\t<string>/home/ana/media server</string>",
		"<key>RunAtLoad</key>\n\t<true/>",
		"<key>SuccessfulExit</key>\n\t\t<false/>",
		"<key>StandardErrorPath</key>\n\t<string>/Users/ana/Library/Logs/mediaserver.log</string>",
	} {
		if !strings.Contains(p, w) {
			t.Errorf("plist lacks %q:\n%s", w, p)
		}
	}
}
