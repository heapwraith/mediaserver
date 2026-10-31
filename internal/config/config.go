// Package config loads the server configuration.
//
// Priority (highest first): command-line flags > environment variables >
// config.toml > built-in defaults.
package config

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// DefaultConfigFile is read when no config file is given explicitly.
// It is optional: a missing default file is not an error.
const DefaultConfigFile = "config.toml"

// Allowed values for enum-like settings.
var (
	hwAccelModes = []string{"auto", "nvenc", "qsv", "amf", "vaapi", "videotoolbox", "none"}
	envModes     = []string{"dev", "prod"}
	logLevels    = []string{"debug", "info", "warn", "error"}
)

// Config holds every runtime setting of the server.
type Config struct {
	MediaDir    string `toml:"media_dir"`
	DataDir     string `toml:"data_dir"`
	Host        string `toml:"host"`
	Port        int    `toml:"port"`
	FFmpegPath  string `toml:"ffmpeg_path"`
	FFprobePath string `toml:"ffprobe_path"`
	HWAccel     string `toml:"hwaccel"`
	Env         string `toml:"env"`
	LogLevel    string `toml:"log_level"`
	ScanWorkers int    `toml:"scan_workers"`  // 0 = number of CPUs
	Watch       bool   `toml:"watch"`         // watch MEDIA_DIR for changes
	ScanOnStart bool   `toml:"scan_on_start"` // incremental scan at startup
	// MaxTranscodes limits the concurrent video encodes (remuxes are free).
	MaxTranscodes int `toml:"max_transcodes"`
	// TranscodeCacheMB is the size of DATA_DIR/transcode (oldest deleted first).
	TranscodeCacheMB int `toml:"transcode_cache_mb"`
}

// Default returns the built-in defaults.
func Default() Config {
	return Config{
		DataDir:     "./data",
		Host:        "0.0.0.0",
		Port:        8080,
		FFmpegPath:  "ffmpeg",
		FFprobePath: "ffprobe",
		HWAccel:     "auto",
		Env:         "prod",
		LogLevel:    "info",
		Watch:       true,
		ScanOnStart: true,

		MaxTranscodes:    2,
		TranscodeCacheMB: 10240,
	}
}

// Addr returns the listen address (host:port).
func (c Config) Addr() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

// ErrHelp is returned by Load when -h or -help was requested.
var ErrHelp = flag.ErrHelp

// Options are extra results of Load that are not part of the config itself.
type Options struct {
	ShowVersion bool
}

// Load builds the configuration from args (without the program name), the
// environment (through getenv) and the config file, then validates it.
func Load(args []string, getenv func(string) string, output io.Writer) (Config, Options, error) {
	cfg := Default()
	var opts Options

	fv := Default() // flag targets; only the ones actually set are applied
	var configFile string
	fset := flag.NewFlagSet("mediaserver", flag.ContinueOnError)
	fset.SetOutput(output)
	fset.StringVar(&configFile, "config", "", "path to config.toml (env CONFIG_FILE)")
	fset.StringVar(&fv.MediaDir, "media-dir", "", "media folder (env MEDIA_DIR)")
	fset.StringVar(&fv.DataDir, "data-dir", fv.DataDir, "data folder: database and caches (env DATA_DIR)")
	fset.StringVar(&fv.Host, "host", fv.Host, "listen host (env HOST)")
	fset.IntVar(&fv.Port, "port", fv.Port, "listen port (env PORT)")
	fset.StringVar(&fv.FFmpegPath, "ffmpeg", fv.FFmpegPath, "ffmpeg binary (env FFMPEG_PATH)")
	fset.StringVar(&fv.FFprobePath, "ffprobe", fv.FFprobePath, "ffprobe binary (env FFPROBE_PATH)")
	fset.StringVar(&fv.HWAccel, "hwaccel", fv.HWAccel, "hardware acceleration: auto|nvenc|qsv|amf|vaapi|videotoolbox|none (env HWACCEL)")
	fset.StringVar(&fv.Env, "env", fv.Env, "environment: dev|prod (env APP_ENV)")
	fset.StringVar(&fv.LogLevel, "log-level", fv.LogLevel, "log level: debug|info|warn|error (env LOG_LEVEL)")
	fset.IntVar(&fv.ScanWorkers, "scan-workers", fv.ScanWorkers, "concurrent hash/probe workers, 0 = CPUs (env SCAN_WORKERS)")
	fset.BoolVar(&fv.Watch, "watch", fv.Watch, "watch the media folder for changes (env WATCH)")
	fset.BoolVar(&fv.ScanOnStart, "scan-on-start", fv.ScanOnStart, "scan the media folder at startup (env SCAN_ON_START)")
	fset.IntVar(&fv.MaxTranscodes, "max-transcodes", fv.MaxTranscodes, "concurrent video transcodes (env MAX_TRANSCODES)")
	fset.IntVar(&fv.TranscodeCacheMB, "transcode-cache-mb", fv.TranscodeCacheMB, "transcode cache size in MB (env TRANSCODE_CACHE_MB)")
	fset.BoolVar(&opts.ShowVersion, "version", false, "print version and exit")
	if err := fset.Parse(args); err != nil {
		return cfg, opts, err
	}
	if opts.ShowVersion {
		return cfg, opts, nil
	}

	// 1. config file
	explicit := true
	if configFile == "" {
		configFile = getenv("CONFIG_FILE")
	}
	if configFile == "" {
		configFile, explicit = DefaultConfigFile, false
	}
	if err := loadFile(&cfg, configFile, explicit); err != nil {
		return cfg, opts, err
	}

	// 2. environment
	if err := applyEnv(&cfg, getenv); err != nil {
		return cfg, opts, err
	}

	// 3. flags (only the ones explicitly set)
	fset.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "media-dir":
			cfg.MediaDir = fv.MediaDir
		case "data-dir":
			cfg.DataDir = fv.DataDir
		case "host":
			cfg.Host = fv.Host
		case "port":
			cfg.Port = fv.Port
		case "ffmpeg":
			cfg.FFmpegPath = fv.FFmpegPath
		case "ffprobe":
			cfg.FFprobePath = fv.FFprobePath
		case "hwaccel":
			cfg.HWAccel = fv.HWAccel
		case "env":
			cfg.Env = fv.Env
		case "log-level":
			cfg.LogLevel = fv.LogLevel
		case "scan-workers":
			cfg.ScanWorkers = fv.ScanWorkers
		case "watch":
			cfg.Watch = fv.Watch
		case "scan-on-start":
			cfg.ScanOnStart = fv.ScanOnStart
		case "max-transcodes":
			cfg.MaxTranscodes = fv.MaxTranscodes
		case "transcode-cache-mb":
			cfg.TranscodeCacheMB = fv.TranscodeCacheMB
		}
	})

	if err := cfg.Validate(); err != nil {
		return cfg, opts, err
	}
	return cfg, opts, nil
}

func loadFile(cfg *Config, path string, explicit bool) error {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if !explicit && errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read config %s: %w", path, err)
	}
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(cfg); err != nil {
		return fmt.Errorf("parse config %s: %w", path, err)
	}
	return nil
}

func applyEnv(cfg *Config, getenv func(string) string) error {
	str := map[string]*string{
		"MEDIA_DIR":    &cfg.MediaDir,
		"DATA_DIR":     &cfg.DataDir,
		"HOST":         &cfg.Host,
		"FFMPEG_PATH":  &cfg.FFmpegPath,
		"FFPROBE_PATH": &cfg.FFprobePath,
		"HWACCEL":      &cfg.HWAccel,
		"APP_ENV":      &cfg.Env,
		"LOG_LEVEL":    &cfg.LogLevel,
	}
	for key, dst := range str {
		if v := getenv(key); v != "" {
			*dst = v
		}
	}
	ints := map[string]*int{
		"PORT": &cfg.Port, "SCAN_WORKERS": &cfg.ScanWorkers,
		"MAX_TRANSCODES": &cfg.MaxTranscodes, "TRANSCODE_CACHE_MB": &cfg.TranscodeCacheMB,
	}
	for key, dst := range ints {
		if v := getenv(key); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("env %s=%q: %w", key, v, err)
			}
			*dst = n
		}
	}
	bools := map[string]*bool{"WATCH": &cfg.Watch, "SCAN_ON_START": &cfg.ScanOnStart}
	for key, dst := range bools {
		if v := getenv(key); v != "" {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("env %s=%q: %w", key, v, err)
			}
			*dst = b
		}
	}
	return nil
}

// Validate checks the configuration and normalizes MediaDir and DataDir to
// absolute paths.
func (c *Config) Validate() error {
	if c.MediaDir == "" {
		return errors.New("MEDIA_DIR is required (flag -media-dir, env MEDIA_DIR or media_dir in config.toml)")
	}
	abs, err := filepath.Abs(c.MediaDir)
	if err != nil {
		return fmt.Errorf("MEDIA_DIR %s: %w", c.MediaDir, err)
	}
	c.MediaDir = abs
	if err := checkReadableDir(c.MediaDir); err != nil {
		return fmt.Errorf("MEDIA_DIR: %w", err)
	}

	if c.DataDir == "" {
		return errors.New("DATA_DIR must not be empty")
	}
	if c.DataDir, err = filepath.Abs(c.DataDir); err != nil {
		return fmt.Errorf("DATA_DIR: %w", err)
	}

	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port %d: must be 1-65535", c.Port)
	}
	if c.ScanWorkers < 0 || c.ScanWorkers > 64 {
		return fmt.Errorf("invalid SCAN_WORKERS %d: must be 0-64 (0 = number of CPUs)", c.ScanWorkers)
	}
	if c.MaxTranscodes < 1 || c.MaxTranscodes > 16 {
		return fmt.Errorf("invalid MAX_TRANSCODES %d: must be 1-16", c.MaxTranscodes)
	}
	if c.TranscodeCacheMB < 512 {
		return fmt.Errorf("invalid TRANSCODE_CACHE_MB %d: must be at least 512", c.TranscodeCacheMB)
	}
	if !slices.Contains(hwAccelModes, c.HWAccel) {
		return fmt.Errorf("invalid HWACCEL %q: want one of %v", c.HWAccel, hwAccelModes)
	}
	if !slices.Contains(envModes, c.Env) {
		return fmt.Errorf("invalid APP_ENV %q: want one of %v", c.Env, envModes)
	}
	if !slices.Contains(logLevels, c.LogLevel) {
		return fmt.Errorf("invalid LOG_LEVEL %q: want one of %v", c.LogLevel, logLevels)
	}
	return nil
}

// checkReadableDir reports an error unless path is an existing, readable
// directory.
func checkReadableDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Readdirnames(1); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

// envFlags maps each environment variable to its flag (Load reads both).
var envFlags = []struct{ env, flag string }{
	{"CONFIG_FILE", "config"},
	{"MEDIA_DIR", "media-dir"},
	{"DATA_DIR", "data-dir"},
	{"HOST", "host"},
	{"PORT", "port"},
	{"FFMPEG_PATH", "ffmpeg"},
	{"FFPROBE_PATH", "ffprobe"},
	{"HWACCEL", "hwaccel"},
	{"APP_ENV", "env"},
	{"LOG_LEVEL", "log-level"},
	{"SCAN_WORKERS", "scan-workers"},
	{"WATCH", "watch"},
	{"SCAN_ON_START", "scan-on-start"},
	{"MAX_TRANSCODES", "max-transcodes"},
	{"TRANSCODE_CACHE_MB", "transcode-cache-mb"},
}

// EnvAsFlags returns, as flags, the settings made through environment
// variables: a service started by the system does not inherit the shell's
// environment (`mediaserver service install`). Flags already in args win.
func EnvAsFlags(args []string, getenv func(string) string) []string {
	var out []string
	for _, e := range envFlags {
		v := getenv(e.env)
		if v == "" || HasFlag(args, e.flag) {
			continue
		}
		out = append(out, "-"+e.flag+"="+v)
	}
	return out
}

// HasFlag reports whether args set the flag name (-name, --name, -name=…).
func HasFlag(args []string, name string) bool {
	for _, a := range args {
		a = strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-")
		if a == name || strings.HasPrefix(a, name+"=") {
			return true
		}
	}
	return false
}
