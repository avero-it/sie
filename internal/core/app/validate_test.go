package app

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/avero-it/sie/internal/core/ports"
)

// fakeValidator reports "BAD" as a problem at its line and character
// column; "LINEONLY" without a column; "NOWHERE" without a position.
type fakeValidator struct{}

func (fakeValidator) Languages() []string { return []string{"JSON", "YAML"} }

func (fakeValidator) Validate(lang string, text []byte) (*ports.Problem, error) {
	for i, line := range strings.Split(string(text), "\n") {
		if at := strings.Index(line, "BAD"); at >= 0 {
			return &ports.Problem{Line: i + 1, Col: utf8.RuneCountInString(line[:at]) + 1, Message: "bad thing"}, nil
		}
		if strings.Contains(line, "LINEONLY") {
			return &ports.Problem{Line: i + 1, Message: "somewhere here"}, nil
		}
		if strings.Contains(line, "NOWHERE") {
			return &ports.Problem{Message: "lost"}, nil
		}
	}
	return nil, nil
}

func newValidateHarness(t *testing.T, files map[string]string) *harness {
	t.Helper()
	m := &memFS{files: map[string][]byte{}}
	for k, v := range files {
		m.files[k] = []byte(v)
	}
	e, err := New(Config{Files: m, Keymap: keys, Version: "test", Validator: fakeValidator{},
		Highlighter: &fakeHighlighter{}, SyntaxHighlighting: true})
	if err != nil {
		t.Fatal(err)
	}
	e.Resize(70, 10)
	return &harness{Editor: e, t: t, fs: m}
}

func TestValidateReportsAndJumps(t *testing.T) {
	h := newValidateHarness(t, map[string]string{"a.yaml": "ok: 1\n  ü: BAD\n"})
	_ = h.Dispatch("file.open", "a.yaml")
	h.keys(ports.Ctrl('t'))
	h.wantRow(rowPrompt, "Validate: line 2, col 6: bad thing")
	if h.styleAt(0, rowPrompt) != ports.StyleError {
		t.Error("problem not shown as an error")
	}
	if line, col, _ := h.doc.CursorPos(); line != 1 || col != len("  ü: ") {
		t.Errorf("cursor at line %d byte col %d", line, col)
	}
}

func TestValidateValidAndUnsupported(t *testing.T) {
	h := newValidateHarness(t, map[string]string{"a.yaml": "ok: 1\n", "notes": "x"})
	_ = h.Dispatch("file.open", "a.yaml")
	h.keys(ports.Ctrl('t'))
	h.wantRowPrefix(rowPrompt, "> Validate — ✓ valid YAML")

	_ = h.Dispatch("file.open", "notes")
	h.keys(ports.Ctrl('t'))
	h.wantRow(rowPrompt, "Validate: cannot validate plain text (supported: JSON, YAML)")

	// The language can be set by hand.
	h.fakeHL().known["JSON"] = true
	_ = h.Dispatch("view.setLanguage", "json")
	h.keys(ports.Ctrl('t'))
	h.wantRowPrefix(rowPrompt, "> Validate — ✓ valid JSON")
}

func TestValidateWithoutColumnOrPosition(t *testing.T) {
	h := newValidateHarness(t, map[string]string{"a.yaml": "a\n  LINEONLY\n", "b.yaml": "x\nNOWHERE\n"})
	_ = h.Dispatch("file.open", "a.yaml")
	h.keys(ports.Ctrl('t'))
	h.wantRow(rowPrompt, "Validate: line 2: somewhere here")
	if line, col, _ := h.doc.CursorPos(); line != 1 || col != 0 {
		t.Errorf("cursor %d:%d, want line start", line, col)
	}
	_ = h.Dispatch("file.open", "b.yaml")
	h.keys(ports.KeyEvent{Key: ports.KeyRight})
	h.keys(ports.Ctrl('t'))
	h.wantRow(rowPrompt, "Validate: lost (position unknown)")
	if h.doc.Selection().Head != 1 {
		t.Error("cursor moved without a position")
	}
}

func TestValidateUnavailable(t *testing.T) {
	h := newHarness(t, nil)
	h.keys(ports.Ctrl('t'))
	h.wantRow(rowPrompt, "Validate: validation is not available")
}

func TestCloseFile(t *testing.T) {
	h := newHarness(t, map[string]string{"a.txt": "hello"})
	_ = h.Dispatch("file.open", "a.txt")
	h.keys(ports.Ctrl('w'))
	if h.path != "" || h.doc.Text() != "" {
		t.Fatalf("not closed: %q %q", h.path, h.doc.Text())
	}
	h.wantRowPrefix(rowPrompt, "> Close file")

	_ = h.Dispatch("file.open", "a.txt")
	h.typeText("x")
	h.keys(ports.Ctrl('w'))
	h.wantRow(rowPrompt, "Close without saving? (y/n)")
	h.typeText("n")
	if h.path != "a.txt" {
		t.Fatal("closed despite answering no")
	}
	h.keys(ports.Ctrl('w'))
	h.typeText("y")
	if h.path != "" || h.doc.Modified() {
		t.Error("not closed after confirming")
	}
}

func (h *harness) fakeHL() *fakeHighlighter {
	fh := h.cfg.Highlighter.(*fakeHighlighter)
	if fh.known == nil {
		fh.known = map[string]bool{}
	}
	return fh
}
