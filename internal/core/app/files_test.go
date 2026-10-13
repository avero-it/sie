package app

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
)

// abs builds an absolute path from a slash path on any OS.
func abs(p string) string {
	p = filepath.FromSlash(p)
	if runtime.GOOS == "windows" {
		return `C:` + p
	}
	return p
}

// newBrowseHarness runs in /w/proj with home /w.
func newBrowseHarness(t *testing.T, files map[string]string) *harness {
	t.Helper()
	m := &memFS{files: map[string][]byte{}, work: abs("/w/proj"), listErr: map[string]error{}}
	for k, v := range files {
		m.files[k] = []byte(v)
	}
	e, err := New(Config{Files: m, Keymap: keys, Version: "test", WorkDir: abs("/w/proj"), HomeDir: abs("/w")})
	if err != nil {
		t.Fatal(err)
	}
	e.Resize(60, 20)
	return &harness{Editor: e, t: t, fs: m}
}

const browseRowPrompt = 18 // prompt row on the 60x20 screen

func labels(h *harness) []string {
	var out []string
	for _, it := range h.prompt.items {
		out = append(out, strings.TrimSpace(it.label))
	}
	return out
}

var project = map[string]string{
	"main.go": "package main\n", "main_test.go": "", "README.md": "", ".gitignore": "",
	"cmd/sie/main.go": "package main // cmd\n", "docs/": "",
}

func TestBrowseListsWorkingDir(t *testing.T) {
	h := newBrowseHarness(t, project)
	h.keys(ports.Ctrl('o'))
	h.wantRow(browseRowPrompt, "Open:")
	s := sep
	want := []string{".." + s, "cmd" + s, "docs" + s, ".gitignore", "main.go", "main_test.go", "README.md"}
	if got := labels(h); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("entries %v, want %v", got, want)
	}
	h.wantRowContains(browseRowPrompt-len(want), "   .."+s)
	if h.prompt.selected != -1 {
		t.Error("something highlighted before navigating")
	}
}

func TestBrowseTabCompletesAndOpens(t *testing.T) {
	h := newBrowseHarness(t, project)
	h.keys(ports.Ctrl('o'))
	h.typeText("ma")
	if got := labels(h); strings.Join(got, ",") != "main.go,main_test.go" {
		t.Fatalf("filtered %v", got)
	}
	h.keys(key(ports.KeyTab))
	if got := h.prompt.input.String(); got != "main" {
		t.Fatalf("common prefix: %q", got)
	}
	h.typeText(".")
	h.keys(key(ports.KeyTab))
	if got := h.prompt.input.String(); got != "main.go" {
		t.Fatalf("single match: %q", got)
	}
	h.keys(key(ports.KeyEnter))
	if h.focus != focusEditor || h.doc.Text() != "package main\n" {
		t.Fatalf("not opened: focus %v text %q", h.focus, h.doc.Text())
	}
	h.wantRowPrefix(browseRowPrompt, "> Open file main.go — 2 lines")
}

func TestBrowseEnterTakesTypedTextWithoutHighlight(t *testing.T) {
	h := newBrowseHarness(t, project)
	h.keys(ports.Ctrl('o'))
	h.typeText("mai") // matches main.go, but nothing is highlighted
	h.keys(key(ports.KeyEnter))
	h.wantRowPrefix(browseRowPrompt, "> Open file mai — new file")
}

func TestBrowseDirectoriesWithKeys(t *testing.T) {
	h := newBrowseHarness(t, project)
	h.keys(ports.Ctrl('o'))
	h.keys(key(ports.KeyDown), key(ports.KeyDown), key(ports.KeyEnter)) // "../", then "cmd/"
	if got := h.prompt.input.String(); got != "cmd"+sep {
		t.Fatalf("into cmd: %q", got)
	}
	h.typeText("s")
	h.keys(key(ports.KeyTab))
	if got := h.prompt.input.String(); got != "cmd"+sep+"sie"+sep {
		t.Fatalf("tab into dir: %q", got)
	}
	h.keys(key(ports.KeyEnter)) // nothing typed after the separator: stay
	if h.focus != focusPrompt {
		t.Fatal("Enter on a bare directory closed the dialog")
	}
	h.keys(key(ports.KeyDown), key(ports.KeyDown), key(ports.KeyEnter)) // "../", "main.go"
	if h.doc.Text() != "package main // cmd\n" {
		t.Fatalf("opened %q", h.doc.Text())
	}
	if !strings.Contains(h.path, filepath.Join("cmd", "sie", "main.go")) {
		t.Errorf("path %q", h.path)
	}
}

func TestBrowseUpWithBackspaceAndDots(t *testing.T) {
	h := newBrowseHarness(t, project)
	h.keys(ports.Ctrl('o'))
	h.typeText("cmd/sie/")
	h.keys(key(ports.KeyBackspace))
	if got := h.prompt.input.String(); got != "cmd"+sep {
		t.Fatalf("up from cmd/sie/: %q", got)
	}
	h.keys(key(ports.KeyBackspace))
	if got := h.prompt.input.String(); got != "" {
		t.Fatalf("up to the working dir: %q", got)
	}
	h.typeText("..")
	h.keys(key(ports.KeyEnter))
	if got := h.prompt.input.String(); got != "~"+sep {
		t.Fatalf("above the working dir shows ~: %q", got)
	}
	if got := labels(h); !strings.Contains(strings.Join(got, ","), "proj"+sep) {
		t.Errorf("home listing %v", got)
	}
	h.keys(key(ports.KeyBackspace)) // above home: absolute
	if got := h.prompt.input.String(); got != abs("/")+"" && !strings.HasSuffix(got, sep) {
		t.Errorf("root: %q", got)
	}
}

func TestBrowseMouseClick(t *testing.T) {
	h := newBrowseHarness(t, project)
	h.keys(ports.Ctrl('o'))
	box := h.completionRect()
	h.click(5, box.y+1, 0) // second row: "cmd/"
	if got := h.prompt.input.String(); got != "cmd"+sep {
		t.Fatalf("click on dir: %q", got)
	}
	box = h.completionRect()
	h.click(5, box.y+1, 0) // "sie/"
	box = h.completionRect()
	h.click(5, box.y+1, 0) // "main.go"
	if h.doc.Text() != "package main // cmd\n" {
		t.Errorf("click on file: %q", h.doc.Text())
	}
}

func TestBrowseHomeExpansionAndErrors(t *testing.T) {
	h := newBrowseHarness(t, nil)
	h.fs.files[abs("/w/notes.txt")] = []byte("n")
	h.keys(ports.CtrlAlt('p'))
	h.typeText("open ~/notes.txt")
	h.keys(key(ports.KeyEnter))
	if h.doc.Text() != "n" {
		t.Fatalf("~ not expanded: %q (path %q)", h.doc.Text(), h.path)
	}
	// The dialog starts in the current file's directory, ~ (= /w).
	h.fs.listErr[abs("/w")] = errors.New("permission denied")
	h.keys(ports.Ctrl('o'))
	if got := h.prompt.input.String(); got != "~"+sep {
		t.Fatalf("start dir %q", got)
	}
	h.typeText("x")
	if got := labels(h); len(got) == 0 || !strings.Contains(strings.Join(got, ","), "permission denied") {
		t.Fatalf("error not listed: %v", got)
	}
	h.keys(key(ports.KeyDown), key(ports.KeyEnter)) // the error line is not selectable
	if h.focus != focusPrompt {
		t.Error("Enter on the error line acted")
	}
}

func TestSaveAsBrowses(t *testing.T) {
	h := newBrowseHarness(t, project)
	h.keys(ports.Ctrl('s')) // unnamed: Save as
	h.wantRowPrefix(browseRowPrompt, "Save as:")
	if len(h.prompt.items) == 0 {
		t.Fatal("Save as does not browse")
	}
	h.typeText("docs/new.txt")
	h.keys(key(ports.KeyEnter))
	if _, ok := h.fs.files["docs/new.txt"]; !ok {
		t.Fatalf("not written: %v", h.fs.files)
	}
	h.keys(ports.CtrlAlt('p'))
	h.typeText("save as")
	h.keys(key(ports.KeyEnter))
	if got := h.prompt.input.String(); got != filepath.Join("docs", "new.txt") {
		t.Errorf("Save as default %q", got)
	}
}

func TestPathHelpers(t *testing.T) {
	h := newBrowseHarness(t, nil)
	tests := []struct{ in, resolved, display string }{
		{"a.txt", "/w/proj/a.txt", "a.txt"},
		{"~/x", "/w/x", "~/x"},
		{"../other/y", "/w/other/y", "~/other/y"},
		{"/etc/hosts", "/etc/hosts", "/etc/hosts"},
		{"", "/w/proj", ""},
	}
	for _, tt := range tests {
		in := tt.in
		if strings.HasPrefix(in, "/") {
			in = abs(in)
		}
		r := h.resolvePath(filepath.FromSlash(in))
		if r != abs(tt.resolved) {
			t.Errorf("resolve(%q) = %q, want %q", tt.in, r, abs(tt.resolved))
		}
		want := filepath.FromSlash(tt.display)
		if strings.HasPrefix(tt.display, "/") {
			want = abs(tt.display)
		}
		if d := h.displayPath(r); d != want {
			t.Errorf("display(%q) = %q, want %q", r, d, want)
		}
	}
}
