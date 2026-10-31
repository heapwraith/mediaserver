package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// newLogger returns a text logger in dev and a JSON logger in prod.
// level must be one of debug, info, warn or error (validated by config).
func newLogger(w io.Writer, env, level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	if env == "dev" {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

// maxLogBytes: a log file past this size starts over (no rotation needed for
// a home server; the service managers keep their own logs on Linux/macOS).
const maxLogBytes = 10 << 20

// openLogFile opens DATA_DIR/mediaserver.log for appending, emptied when it
// grew past maxLogBytes.
func openLogFile(dataDir string) (*os.File, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "mediaserver.log")
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogBytes {
		flags |= os.O_TRUNC
	}
	return os.OpenFile(path, flags, 0o600) //nolint:gosec // path built from DATA_DIR
}
