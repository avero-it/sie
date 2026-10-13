// Package buildinfo reports how the sie binary was built. Values come from
// -ldflags when set, otherwise from the build information the Go toolchain
// embeds (module version, VCS revision and time):
//
//	go build -ldflags "\
//	  -X github.com/avero-it/sie/internal/buildinfo.version=v0.1.0 \
//	  -X github.com/avero-it/sie/internal/buildinfo.commit=$(git rev-parse HEAD) \
//	  -X github.com/avero-it/sie/internal/buildinfo.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" ./cmd/sie
package buildinfo

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// Set with -ldflags "-X github.com/avero-it/sie/internal/buildinfo.<name>=<value>".
var (
	version string
	commit  string
	date    string // build time, RFC 3339
)

// Info describes the running binary.
type Info struct {
	Version    string
	Commit     string
	Modified   bool   // built from a working tree with uncommitted changes
	CommitTime string // RFC 3339, from the VCS
	BuildTime  string // RFC 3339
	BuildSrc   string // where BuildTime comes from: "ldflags", "file time" or ""
	GoVersion  string
	Compiler   string
	Platform   string // GOOS/GOARCH
}

// Get collects the build information.
func Get() Info {
	info := Info{
		Version:   version,
		Commit:    commit,
		BuildTime: date,
		GoVersion: runtime.Version(),
		Compiler:  runtime.Compiler,
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if info.Version == "" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			info.Version = bi.Main.Version
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if info.Commit == "" {
					info.Commit = s.Value
				}
			case "vcs.time":
				info.CommitTime = s.Value
			case "vcs.modified":
				info.Modified = s.Value == "true"
			}
		}
	}
	if info.Version == "" {
		info.Version = "dev"
	}
	switch {
	case info.BuildTime != "":
		info.BuildSrc = "ldflags"
	default:
		// Go does not record the build time; the binary's modification
		// time is the best approximation without -ldflags.
		if exe, err := os.Executable(); err == nil {
			if st, err := os.Stat(exe); err == nil {
				info.BuildTime = st.ModTime().UTC().Format(time.RFC3339)
				info.BuildSrc = "file time"
			}
		}
	}
	return info
}

// Short is a one-line version, e.g. "v0.1.0 (3aea623)".
func (i Info) Short() string {
	// Pseudo-versions stamped by the go command already embed the commit.
	if i.Commit == "" || strings.Contains(i.Version, i.Commit[:min(12, len(i.Commit))]) {
		return i.Version
	}
	c := i.Commit[:min(7, len(i.Commit))]
	if i.Modified {
		c += "+dirty"
	}
	return fmt.Sprintf("%s (%s)", i.Version, c)
}

// String is the multi-line report printed by "sie version".
func (i Info) String() string {
	var b strings.Builder
	row := func(k, v string) {
		if v == "" {
			v = "unknown"
		}
		fmt.Fprintf(&b, "  %-12s %s\n", k+":", v)
	}
	fmt.Fprintf(&b, "sie %s\n", i.Version)
	c := i.Commit
	if c != "" && i.Modified {
		c += " (uncommitted changes)"
	}
	row("commit", c)
	row("commit time", i.CommitTime)
	bt := i.BuildTime
	if bt != "" && i.BuildSrc == "file time" {
		bt += " (binary file time)"
	}
	row("built", bt)
	row("go", i.GoVersion+" ("+i.Compiler+")")
	row("platform", i.Platform)
	return b.String()
}
