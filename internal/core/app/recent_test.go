package app

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
)

func newRecentHarness(t *testing.T) (*harness, *memRecent) {
	t.Helper()
	m := &memFS{files: map[string][]byte{}, work: abs("/w/proj"), listErr: map[string]error{}}
	for _, p := range []string{"/w/proj/main.go", "/w/proj/cmd/sie/main.go", "/w/proj/README.md", "/w/notes/todo.md"} {
		m.files[abs(p)] = []byte(p)
	}
	rec := &memRecent{}
	e, err := New(Config{Files: m, Keymap: keys, Version: "test", WorkDir: abs("/w/proj"), HomeDir: abs("/w"), Recent: rec})
	if err != nil {
		t.Fatal(err)
	}
	e.Resize(60, 20)
	return &harness{Editor: e, t: t, fs: m}, rec
}

func TestOpenAndSaveAreRecorded(t *testing.T) {
	h, rec := newRecentHarness(t)
	_ = h.Dispatch("file.open", abs("/w/proj/main.go"))
	_ = h.Dispatch("file.open", "~/notes/todo.md")
	_ = h.Dispatch("file.open", "new.txt") // new file: not recorded until saved
	want := []string{abs("/w/notes/todo.md"), abs("/w/proj/main.go")}
	if !slices.Equal(rec.files, want) {
		t.Fatalf("recorded %v, want %v", rec.files, want)
	}
	h.typeText("x")
	h.keys(ports.Ctrl('s'))
	if rec.files[0] != abs("/w/proj/new.txt") {
		t.Errorf("save not recorded: %v", rec.files)
	}
}

func TestOpenRecentListAndFuzzyPick(t *testing.T) {
	h, rec := newRecentHarness(t)
	rec.files = []string{
		abs("/w/proj/README.md"), abs("/w/notes/todo.md"), abs("/w/proj/cmd/sie/main.go"),
		abs("/w/proj/gone.go"), // no longer exists: hidden
	}
	_ = h.Dispatch("file.open", "README.md") // the current file is not listed
	h.keys(ports.Ctrl('e'))
	h.wantRowPrefix(browseRowPrompt, "Open recent:")
	got := labels(h)
	want := []string{filepath.FromSlash("~/notes/todo.md"), filepath.FromSlash("cmd/sie/main.go")}
	if !slices.Equal(got, want) {
		t.Fatalf("list %v, want %v", got, want)
	}
	if h.prompt.selected != 0 {
		t.Error("first entry not ready for Enter")
	}
	h.typeText("csm") // fuzzy: cmd/sie/main.go
	if got := labels(h); len(got) == 0 || got[0] != filepath.FromSlash("cmd/sie/main.go") {
		t.Fatalf("filtered %v", got)
	}
	h.keys(key(ports.KeyEnter))
	if h.doc.Text() != "/w/proj/cmd/sie/main.go" {
		t.Fatalf("opened %q", h.doc.Text())
	}
	h.wantRowPrefix(browseRowPrompt, "> Open recent "+filepath.FromSlash("cmd/sie/main.go"))
	if rec.files[0] != abs("/w/proj/cmd/sie/main.go") {
		t.Errorf("reopened file not moved to the top: %v", rec.files)
	}
}

func TestOpenRecentKeysAndEmpty(t *testing.T) {
	h, rec := newRecentHarness(t)
	h.keys(ports.Ctrl('e'))
	h.keys(key(ports.KeyEnter))
	h.wantRow(browseRowPrompt, "No recent files")

	rec.files = []string{abs("/w/proj/main.go"), abs("/w/notes/todo.md")}
	h.keys(ports.Ctrl('e'))
	h.keys(key(ports.KeyDown), key(ports.KeyTab)) // Tab copies the highlighted path
	if got := h.prompt.input.String(); got != filepath.FromSlash("~/notes/todo.md") {
		t.Fatalf("tab: %q", got)
	}
	h.keys(key(ports.KeyEsc))
	h.keys(ports.Ctrl('e'))
	h.typeText("zzz")
	h.keys(key(ports.KeyEnter))
	h.wantRow(browseRowPrompt, `no recent file matches "zzz"`)

	// Mouse: a click opens the entry.
	h.keys(ports.Ctrl('e'))
	box := h.completionRect()
	h.click(5, box.y+1, 0)
	if h.doc.Text() != "/w/notes/todo.md" {
		t.Errorf("click opened %q", h.doc.Text())
	}
}

func TestOpenRecentInline(t *testing.T) {
	h, rec := newRecentHarness(t)
	rec.files = []string{abs("/w/proj/main.go"), abs("/w/notes/todo.md")}
	h.keys(ports.CtrlAlt('p'))
	h.typeText("recent todo")
	h.keys(key(ports.KeyEnter))
	if h.doc.Text() != "/w/notes/todo.md" {
		t.Errorf("inline best match opened %q", h.doc.Text())
	}
	if err := h.Dispatch("file.openRecent", "nothing-like-it"); err == nil || !strings.Contains(err.Error(), "no recent file") {
		t.Errorf("err %v", err)
	}
}

func TestOpenRecentUnavailable(t *testing.T) {
	h := newHarness(t, nil) // no RecentFiles configured
	h.keys(ports.Ctrl('e'))
	if got := labels(h); len(got) != 1 || !strings.Contains(got[0], "not available") {
		t.Errorf("labels %v", got)
	}
}
