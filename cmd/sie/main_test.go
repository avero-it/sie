package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/adapters/keymap"
	"github.com/avero-it/sie/internal/core/app"
)

func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersionCommand(t *testing.T) {
	code, out, errOut := runCLI(t, "version")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, want := range []string{"sie ", "commit:", "commit time:", "built:", "go:          " + runtime.Version(), "platform:"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestVersionFlag(t *testing.T) {
	code, out, _ := runCLI(t, "--version")
	if code != 0 || !strings.HasPrefix(out, "sie version ") {
		t.Errorf("exit %d, output %q", code, out)
	}
}

func TestTooManyArgs(t *testing.T) {
	code, _, errOut := runCLI(t, "a", "b")
	if code != 1 || !strings.Contains(errOut, "sie: accepts at most 1 arg") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestConfigCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(path, []byte("tab_width: 2\nshow_invisibles: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCLI(t, "--config", path, "config")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, want := range []string{"# from " + path, "tab_width: 2", "show_invisibles: true", "show_empty_cells: true", "click_ripple: false", "indent_guide: false", "wrap: false", "line_ending: lf", "jump_lines: 5", "syntax_highlighting: true", "syntax_max_size: 2MB", "validate_on_save: true", "large_file_size: 32MB"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestBadConfigFailsBeforeTerminal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(path, []byte("tab_width: 99\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runCLI(t, "--config", path, "file.txt")
	if code != 1 || !strings.Contains(errOut, "sie: config: tab_width") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

// TestEveryCommandHasAShortcut: whatever can be run from the prompt can also
// be run from the keyboard, with one key or a Ctrl+P chord.
func TestEveryCommandHasAShortcut(t *testing.T) {
	km := keymap.Default()
	for _, id := range app.BuiltinIDs() {
		if km.Shortcut(id) == "" {
			t.Errorf("%s has no shortcut", id)
		}
	}
}
