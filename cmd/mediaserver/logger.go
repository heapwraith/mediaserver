package main

import (
	"io"
	"log/slog"
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
