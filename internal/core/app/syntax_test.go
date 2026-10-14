package app

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/domain/editor"
	"github.com/avero-it/sie/internal/core/ports"
)

// fakeHighlighter records jobs; tests release results explicitly, which
// simulates the background worker finishing later.
type fakeHighlighter struct {
	jobs    []ports.HighlightJob
	results []ports.HighlightResult
	known   map[string]bool // extra languages accepted by Language
	// lex classifies every occurrence of these words.
	words map[string]ports.TokenClass
}

func (f *fakeHighlighter) Detect(name string) string {
	switch filepath.Ext(name) {
	case ".go":
		return "Go"
	case ".yaml", ".yml":
		return "YAML"
	}
	return ""
}

func (f *fakeHighlighter) Language(name string) (string, bool) {
	if f.known[strings.ToUpper(name)] {
		return strings.ToUpper(name), true
	}
	switch strings.ToLower(name) {
	case "go", "golang":
		return "Go", true
	case "yaml", "yml":
		return "YAML", true
	}
	return "", false
}

func (f *fakeHighlighter) Highlight(job ports.HighlightJob) { f.jobs = append(f.jobs, job) }

func (f *fakeHighlighter) Result() (ports.HighlightResult, bool) {
	if len(f.results) == 0 {
		return ports.HighlightResult{}, false
	}
	r := f.results[len(f.results)-1] // newest wins, like the real adapter
	f.results = nil
	return r, true
}

// finish lexes the newest job and makes its result available.
func (f *fakeHighlighter) finish() {
	job := f.jobs[len(f.jobs)-1]
	var tokens []ports.Token
	text := string(job.Text)
	for i := 0; i < len(text); {
		matched := false
		for w, c := range f.words {
			if strings.HasPrefix(text[i:], w) {
				tokens = append(tokens, ports.Token{Start: i, End: i + len(w), Class: c})
				i += len(w)
				matched = true
				break
			}
		}
		if !matched {
			i++
		}
	}
	f.results = append(f.results, ports.HighlightResult{ID: job.ID, Tokens: tokens})
}

func newSyntaxHarness(t *testing.T, files map[string]string) (*harness, *fakeHighlighter) {
	t.Helper()
	fh := &fakeHighlighter{words: map[string]ports.TokenClass{"func": ports.TokenKeyword, `"s"`: ports.TokenString}}
	m := &memFS{files: map[string][]byte{}}
	for k, v := range files {
		m.files[k] = []byte(v)
	}
	e, err := New(Config{Files: m, Keymap: keys, Version: "test", Highlighter: fh, SyntaxHighlighting: true})
	if err != nil {
		t.Fatal(err)
	}
	e.Resize(60, 10)
	return &harness{Editor: e, t: t, fs: m}, fh
}

func TestHighlightAppliesTokens(t *testing.T) {
	h, fh := newSyntaxHarness(t, map[string]string{"a.go": `func f() { x := "s" }`})
	_ = h.Dispatch("file.open", "a.go")
	if len(fh.jobs) != 1 || fh.jobs[0].Language != "Go" {
		t.Fatalf("jobs %+v", fh.jobs)
	}
	if h.styleAt(3, rowText) != ports.StyleDefault {
		t.Error("colored before the result arrived")
	}
	fh.finish()
	h.Poll()
	if h.styleAt(3, rowText) != ports.StyleKeyword || h.styleAt(3+3, rowText) != ports.StyleKeyword {
		t.Error("keyword not colored")
	}
	if h.styleAt(3+17, rowText) != ports.StyleString {
		t.Error("string not colored")
	}
	if h.styleAt(3+5, rowText) != ports.StyleDefault {
		t.Error("plain text colored")
	}
	if !strings.Contains(h.row(rowStatus), "Go   UTF-8") {
		t.Errorf("status %q", h.row(rowStatus))
	}
}

func TestStaleTokensFollowEdits(t *testing.T) {
	h, fh := newSyntaxHarness(t, map[string]string{"a.go": `func f() { "s" }`})
	_ = h.Dispatch("file.open", "a.go")
	fh.finish()
	h.Poll()
	h.doc.SetSelection(editor.Cursor(5)) // after "func "
	h.typeText("XX")                     // edit between the two tokens
	if len(fh.jobs) != 3 {               // one job per keystroke
		t.Fatalf("jobs %d", len(fh.jobs))
	}
	// Before the new result: "func" kept, the string shifted by 2.
	if h.styleAt(3, rowText) != ports.StyleKeyword || h.styleAt(3+13, rowText) != ports.StyleString {
		t.Errorf("stale tokens not mapped: %v %v", h.styleAt(3, rowText), h.styleAt(3+13, rowText))
	}
	// Typing inside a token makes that token plain until re-highlighted.
	h.doc.SetSelection(editor.Cursor(2))
	h.typeText("n")
	if h.styleAt(3, rowText) != ports.StyleDefault {
		t.Error("edited token still colored")
	}
	fh.finish()
	h.Poll()
	if h.styleAt(3+14, rowText) != ports.StyleString {
		t.Error("fresh result not applied")
	}
}

func TestOldResultsIgnored(t *testing.T) {
	h, fh := newSyntaxHarness(t, map[string]string{"a.go": `func`, "b.go": `x "s"`})
	_ = h.Dispatch("file.open", "a.go")
	fh.finish() // result for a.go, not yet collected
	_ = h.Dispatch("file.open", "b.go")
	h.Poll() // the a.go result must not color b.go
	if h.styleAt(3, rowText) != ports.StyleDefault {
		t.Error("result of the previous document applied")
	}
	fh.finish()
	h.Poll()
	if h.styleAt(3+2, rowText) != ports.StyleString {
		t.Error("b.go not highlighted")
	}
	// Errors are ignored.
	fh.results = []ports.HighlightResult{{ID: fh.jobs[len(fh.jobs)-1].ID, Err: errors.New("boom")}}
	h.Poll()
}

func TestToggleAndSetLanguage(t *testing.T) {
	h, fh := newSyntaxHarness(t, map[string]string{"notes": `func`})
	_ = h.Dispatch("file.open", "notes")
	if len(fh.jobs) != 0 || !strings.Contains(h.row(rowStatus), "Plain") {
		t.Fatalf("unknown extension: jobs %d status %q", len(fh.jobs), h.row(rowStatus))
	}
	_ = h.Dispatch("view.setLanguage", "golang")
	h.wantRowPrefix(rowPrompt, "> Set language golang — Go")
	fh.finish()
	h.Poll()
	if h.styleAt(3, rowText) != ports.StyleKeyword {
		t.Error("manual language not highlighted")
	}
	// A manual language survives Save as.
	_ = h.Dispatch("file.saveAs", "notes.yaml")
	if h.hl.language != "Go" {
		t.Errorf("language after Save as: %q", h.hl.language)
	}
	if err := h.Dispatch("view.setLanguage", "klingon"); err == nil {
		t.Error("unknown language accepted")
	}
	_ = h.Dispatch("view.toggleSyntax")
	h.wantRowPrefix(rowPrompt, ">   Syntax highlighting — hidden")
	if h.styleAt(3, rowText) != ports.StyleDefault {
		t.Error("colors shown with highlighting off")
	}
	_ = h.Dispatch("view.setLanguage", "plain")
	if h.hl.language != "" || !strings.Contains(h.row(rowStatus), "Plain") {
		t.Errorf("plain: %q", h.hl.language)
	}
}

func TestSelectionAndMarkersWinOverSyntax(t *testing.T) {
	h, fh := newSyntaxHarness(t, map[string]string{"a.go": `func`})
	_ = h.Dispatch("file.open", "a.go")
	fh.finish()
	h.Poll()
	h.keys(ports.Ctrl('a'))
	if h.styleAt(3, rowText) != ports.StyleSelection {
		t.Error("selection lost to syntax color")
	}
}

func TestTooLargeStaysPlain(t *testing.T) {
	fh := &fakeHighlighter{}
	m := &memFS{files: map[string][]byte{"big.go": []byte(strings.Repeat("x", 100))}}
	e, err := New(Config{Files: m, Keymap: keys, Highlighter: fh, SyntaxHighlighting: true, SyntaxMaxSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	e.Resize(80, 10)
	h := &harness{Editor: e, t: t, fs: m}
	_ = h.Dispatch("file.open", "big.go")
	if len(fh.jobs) != 0 {
		t.Error("large file sent to the highlighter")
	}
	if !strings.Contains(h.row(rowStatus), "Go (too large)") {
		t.Errorf("status %q", h.row(rowStatus))
	}
}

func TestShiftTokens(t *testing.T) {
	tk := func(s, e int) ports.Token { return ports.Token{Start: s, End: e, Class: ports.TokenKeyword} }
	got := shiftTokens([]ports.Token{tk(0, 3), tk(5, 8), tk(10, 12)}, 4, 0, 2) // insert 2 at 4
	if len(got) != 3 || got[0] != tk(0, 3) || got[1] != tk(7, 10) || got[2] != tk(12, 14) {
		t.Errorf("insert between: %+v", got)
	}
	got = shiftTokens([]ports.Token{tk(0, 3), tk(5, 8), tk(10, 12)}, 6, 1, 0) // delete inside the 2nd
	if len(got) != 2 || got[1] != tk(9, 11) {
		t.Errorf("delete inside: %+v", got)
	}
}
