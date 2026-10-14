// Package archtest enforces the hexagonal dependency rules from CLAUDE.md as a
// plain `go test`, so the rules hold even without golangci-lint installed.
package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/avero-it/sie"

const (
	domain   = module + "/internal/core/domain"
	app      = module + "/internal/core/app"
	ports    = module + "/internal/core/ports"
	adapters = module + "/internal/adapters/"
	tcell    = "github.com/gdamore/tcell"
	uniseg   = "github.com/rivo/uniseg"
	viper    = "github.com/spf13/viper"
	cobra    = "github.com/spf13/cobra"
	chroma   = "github.com/alecthomas/chroma"
	goyaml   = "github.com/goccy/go-yaml"
	gotoml   = "github.com/pelletier/go-toml"
	cmd      = module + "/cmd/sie"
)

// osBound are stdlib packages that reach the OS; the core must use ports instead.
var osBound = []string{"os", "os/exec", "os/signal", "os/user", "syscall", "net", "plugin", "unsafe"}

func TestDependencyRules(t *testing.T) {
	root := moduleRoot(t)
	files := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// vendor/ is third-party code; cgo in the build graph is checked
			// with `go list -deps` instead (see CLAUDE.md).
			if path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata" ||
				path == filepath.Join(root, "vendor")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		pkg := module + "/" + filepath.ToSlash(filepath.Dir(rel))
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		files++
		for _, spec := range f.Imports {
			imp, _ := strconv.Unquote(spec.Path.Value)
			if msg := violation(pkg, imp); msg != "" {
				t.Errorf("%s imports %q: %s", rel, imp, msg)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("no Go files scanned; module root detection is broken")
	}
}

// violation returns why pkg may not import imp, or "" if allowed.
func violation(pkg, imp string) string {
	if imp == "C" {
		return "cgo is forbidden (CGO_ENABLED=0 everywhere)"
	}
	if strings.HasPrefix(imp, tcell) && !within(pkg, adapters+"tui") {
		return "tcell is allowed only in internal/adapters/tui"
	}
	if within(imp, viper) && !within(pkg, adapters+"config") {
		return "viper is allowed only in internal/adapters/config"
	}
	if within(imp, cobra) && !within(pkg, cmd) {
		return "cobra is allowed only in cmd/sie"
	}
	if strings.HasPrefix(imp, chroma) && !within(pkg, adapters+"syntax") {
		return "chroma is allowed only in internal/adapters/syntax"
	}
	if (strings.HasPrefix(imp, goyaml) || strings.HasPrefix(imp, gotoml)) && !within(pkg, adapters+"validate") {
		return "goccy/go-yaml and go-toml are allowed only in internal/adapters/validate"
	}
	switch {
	case within(pkg, domain):
		return coreRule(imp, domain)
	case within(pkg, app):
		return coreRule(imp, domain, app, ports)
	case within(pkg, ports):
		return coreRule(imp, domain, ports)
	case strings.HasPrefix(pkg, adapters):
		own := adapters + strings.SplitN(strings.TrimPrefix(pkg, adapters), "/", 2)[0]
		if strings.HasPrefix(imp, adapters) && !within(imp, own) {
			return "adapters must not import other adapters; wire them in cmd/sie"
		}
	}
	return ""
}

// coreRule allows stdlib (minus OS-bound packages), uniseg and the given module packages.
func coreRule(imp string, allowed ...string) string {
	for _, a := range allowed {
		if within(imp, a) {
			return ""
		}
	}
	if imp == uniseg {
		return ""
	}
	if isStdlib(imp) {
		for _, bad := range osBound {
			if within(imp, bad) {
				return "core must not touch the OS; go through a port"
			}
		}
		return ""
	}
	return "core may import only stdlib, rivo/uniseg and inner hexagon layers"
}

func within(pkg, prefix string) bool {
	return pkg == prefix || strings.HasPrefix(pkg, prefix+"/")
}

func isStdlib(imp string) bool {
	first, _, _ := strings.Cut(imp, "/")
	return !strings.Contains(first, ".")
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
