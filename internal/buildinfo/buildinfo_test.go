package buildinfo

import (
	"runtime"
	"strings"
	"testing"
)

func TestGetHasRuntimeFacts(t *testing.T) {
	i := Get()
	if i.Version == "" || i.GoVersion != runtime.Version() || i.Platform != runtime.GOOS+"/"+runtime.GOARCH {
		t.Errorf("%+v", i)
	}
}

func TestLdflagsWin(t *testing.T) {
	version, commit, date = "v1.2.3", "0123456789abcdef", "2026-10-13T12:00:00Z"
	t.Cleanup(func() { version, commit, date = "", "", "" })
	i := Get()
	if i.Version != "v1.2.3" || i.Commit != "0123456789abcdef" || i.BuildTime != date || i.BuildSrc != "ldflags" {
		t.Errorf("%+v", i)
	}
}

func TestFormatting(t *testing.T) {
	i := Info{Version: "v1.0.0", Commit: "0123456789abcdef", Modified: true,
		CommitTime: "2026-10-13T10:00:00Z", BuildTime: "2026-10-13T11:00:00Z", BuildSrc: "file time",
		GoVersion: "go1.26.4", Compiler: "gc", Platform: "linux/amd64"}
	if got := i.Short(); got != "v1.0.0 (0123456+dirty)" {
		t.Errorf("Short = %q", got)
	}
	s := i.String()
	for _, want := range []string{
		"sie v1.0.0",
		"commit:      0123456789abcdef (uncommitted changes)",
		"commit time: 2026-10-13T10:00:00Z",
		"built:       2026-10-13T11:00:00Z (binary file time)",
		"go:          go1.26.4 (gc)",
		"platform:    linux/amd64",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if got := (Info{Version: "dev"}).String(); !strings.Contains(got, "commit:      unknown") {
		t.Errorf("unknown fields:\n%s", got)
	}
}

func TestShortPseudoVersion(t *testing.T) {
	i := Info{Version: "v0.0.0-20261013165545-3aea623c41d6+dirty", Commit: "3aea623c41d605409b380af26aa3128b09f8e43d", Modified: true}
	if got := i.Short(); got != i.Version {
		t.Errorf("Short = %q", got)
	}
}
