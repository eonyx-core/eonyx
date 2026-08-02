// Package version exposes the build version of eonyx. The value is the git
// tag, injected at build time via -ldflags; plain `go build` reports "dev" and
// falls back to the module/VCS info embedded by the Go toolchain.
package version

import (
	"runtime/debug"
	"strings"
)

// Version is overridden at build time:
//
//	go build -ldflags="-X eonyx/internal/version.Version=$(git describe --tags --always --dirty)"
var Version = "dev"

// String returns the build version. When -ldflags was not used (e.g. a plain
// local build or `go install eonyx@vX.Y.Z`), it derives a readable value
// from the build info the Go toolchain embeds: a real tag if there is one,
// otherwise "dev-<short commit>[-dirty]". Go's unreadable pseudo-version
// (v0.0.0-<timestamp>-<hash>) is deliberately ignored.
func String() string {
	if Version != "dev" {
		return Version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return Version
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" && !strings.HasPrefix(v, "v0.0.0-") {
		return v
	}

	rev := ""
	dirty := false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) >= 7 {
				rev = s.Value[:7]
			}
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return Version
	}
	out := "dev-" + rev
	if dirty {
		out += "-dirty"
	}
	return out
}
