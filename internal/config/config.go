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
	if v := getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("env PORT=%q: %w", v, err)
		}
		cfg.Port = p
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
