package console

import (
	"embed"
	"io/fs"
)

// dist is the built interface, compiled into the binary so the console stays
// one file to ship.
//
// The pattern must always match something or the build fails, which is why
// ui/dist/.gitkeep is committed while the build output is not. That choice is
// what keeps `go build ./...` working in a fresh clone with no node toolchain
// installed — a property worth more than the convenience of a prebuilt asset,
// since every Go contributor pays for losing it and only the console gains.
//
//go:embed all:ui/dist
var dist embed.FS

// embeddedUI returns the built interface, or false when this binary carries
// none.
//
// A binary without one is a legitimate state, not a broken build: it is what
// `go build` produces before anyone runs `make ui`. So the answer is a boolean
// the caller can act on, and the caller says so out loud rather than serving a
// blank page that looks like a bug in the data.
func embeddedUI() (fs.FS, bool) {
	sub, err := fs.Sub(dist, "ui/dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, false
	}
	return sub, true
}
