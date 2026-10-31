package config

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadPrecedence(t *testing.T) {
	// Paths with spaces, accents and Unicode on purpose (AGENTS.md §5.7).
	root := t.TempDir()
	dirs := map[string]string{}
	for _, name := range []string{"from file", "do ambiente é", "フラグ"} {
		p := filepath.Join(root, name)
		if err := os.Mkdir(p, 0o750); err != nil {
			t.Fatal(err)
		}
		dirs[name] = p
	}
	cfgFile := filepath.Join(root, "config.toml")
	writeFile(t, cfgFile, "media_dir = '"+dirs["from file"]+"'\nport = 9000\nhwaccel = 'nvenc'\n")

	tests := []struct {
		name      string
		args      []string
		env       map[string]string
		wantMedia string
		wantPort  int
		wantHW    string
	}{
		{
			name:      "file only",
			args:      []string{"-config", cfgFile},
			wantMedia: dirs["from file"], wantPort: 9000, wantHW: "nvenc",
		},
		{
			name:      "env overrides file",
			args:      []string{"-config", cfgFile},
			env:       map[string]string{"MEDIA_DIR": dirs["do ambiente é"], "PORT": "9100"},
			wantMedia: dirs["do ambiente é"], wantPort: 9100, wantHW: "nvenc",
		},
		{
			name:      "flags override env and file",
			args:      []string{"-config", cfgFile, "-media-dir", dirs["フラグ"], "-port", "9200", "-hwaccel", "none"},
			env:       map[string]string{"MEDIA_DIR": dirs["do ambiente é"], "PORT": "9100", "HWACCEL": "qsv"},
			wantMedia: dirs["フラグ"], wantPort: 9200, wantHW: "none",
		},
		{
			name:      "unset flag keeps env value",
			args:      []string{"-config", cfgFile, "-port", "9300"},
			env:       map[string]string{"HWACCEL": "vaapi"},
			wantMedia: dirs["from file"], wantPort: 9300, wantHW: "vaapi",
		},
		{
			name:      "config file from env",
			env:       map[string]string{"CONFIG_FILE": cfgFile},
			wantMedia: dirs["from file"], wantPort: 9000, wantHW: "nvenc",
		},
		{
			name:      "defaults without file",
			args:      []string{"-media-dir", dirs["フラグ"]},
			wantMedia: dirs["フラグ"], wantPort: 8080, wantHW: "auto",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(dirs["フラグ"]) // no config.toml here: default file is optional
			cfg, _, err := Load(tt.args, envFrom(tt.env), io.Discard)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.MediaDir != tt.wantMedia || cfg.Port != tt.wantPort || cfg.HWAccel != tt.wantHW {
				t.Errorf("got media=%q port=%d hw=%q, want media=%q port=%d hw=%q",
					cfg.MediaDir, cfg.Port, cfg.HWAccel, tt.wantMedia, tt.wantPort, tt.wantHW)
			}
			if !filepath.IsAbs(cfg.DataDir) {
				t.Errorf("DataDir %q is not absolute", cfg.DataDir)
			}
		})
	}
}

func TestLoadErrors(t *testing.T) {
	root := t.TempDir()
	media := filepath.Join(root, "media")
	if err := os.Mkdir(media, 0o750); err != nil {
		t.Fatal(err)
	}
	notDir := filepath.Join(root, "file.mkv")
	writeFile(t, notDir, "x")
	badToml := filepath.Join(root, "bad.toml")
	writeFile(t, badToml, "unknown_key = 1\n")

	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		wantErr string
	}{
		{"missing media dir", nil, nil, "MEDIA_DIR is required"},
		{"media dir does not exist", []string{"-media-dir", filepath.Join(root, "nope")}, nil, "MEDIA_DIR"},
		{"media dir is a file", []string{"-media-dir", notDir}, nil, "not a directory"},
		{"bad port flag", []string{"-media-dir", media, "-port", "70000"}, nil, "invalid port"},
		{"bad port env", []string{"-media-dir", media}, map[string]string{"PORT": "abc"}, "env PORT"},
		{"bad hwaccel", []string{"-media-dir", media, "-hwaccel", "cuda"}, nil, "invalid HWACCEL"},
		{"bad env", []string{"-media-dir", media, "-env", "staging"}, nil, "invalid APP_ENV"},
		{"bad log level", []string{"-media-dir", media, "-log-level", "trace"}, nil, "invalid LOG_LEVEL"},
		{"explicit config missing", []string{"-config", filepath.Join(root, "none.toml")}, nil, "read config"},
		{"unknown toml key", []string{"-config", badToml, "-media-dir", media}, nil, "parse config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(root)
			_, _, err := Load(tt.args, envFrom(tt.env), io.Discard)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("got err %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadHelpAndVersion(t *testing.T) {
	_, _, err := Load([]string{"-h"}, envFrom(nil), io.Discard)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("got %v, want ErrHelp", err)
	}
	_, opts, err := Load([]string{"-version"}, envFrom(nil), io.Discard)
	if err != nil || !opts.ShowVersion {
		t.Fatalf("got opts=%+v err=%v, want ShowVersion", opts, err)
	}
}
