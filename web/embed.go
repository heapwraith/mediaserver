// Package web embeds the built frontend (web/build) into the Go binary.
package web

import (
	"embed"
	"io/fs"
)

// build is filled by `make build-web`. The Makefile creates an empty
// placeholder so the Go code compiles before the first frontend build.
//
//go:embed all:build
var build embed.FS

// Assets returns the built frontend with index.html at its root.
func Assets() (fs.FS, error) {
	return fs.Sub(build, "build")
}
