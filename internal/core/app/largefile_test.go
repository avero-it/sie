package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
)

const largeLimit = 200 // bytes: files over it stay on disk in these tests

func numbered(lines int) string {
	var sb strings.Builder
	for i := 1; i <= lines; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	return sb.String()
}

func newLargeHarness(t *testing.T, files map[string]string, mod func(*Config)) *harness {
	t.Helper()
	m := &memFS{files: map[string][]byte{}}
	for k, v := range files {
		m.files[k] = []byte(v)
	}
	cfg := Config{Files: m, Keymap: keys, Version: "test", LargeFileSize: largeLimit, Validator: fakeValidator{},
		Highlighter: &fakeHighlighter{}, SyntaxHighlighting: true, ValidateOnSave: true}
	if mod != nil {
		mod(&cfg)
	}
	e, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.Resize(100, 10)
	return &harness{Editor: e, t: t, fs: m}
}

func TestSmallFileIsLoadedAndClosed(t *testing.T) {
	h := newLargeHarness(t, map[string]string{"s.txt": "tiny\n"}, nil)
	_ = h.Dispatch("file.open", "s.txt")
	if h.file != nil || len(h.fs.opened) != 1 || !h.fs.opened[0].closed {
		t.Fatalf("small file should be read and closed: file=%v opened=%+v", h.file, h.fs.opened)
	}
}

func TestLargeFileStaysOnDisk(t *testing.T) {
	text := numbered(500) // 4 KB
	h := newLargeHarness(t, map[string]string{"big.txt": text}, nil)
	_ = h.Dispatch("file.open", "big.txt")
	if h.file == nil || h.fs.opened[0].closed {
		t.Fatal("large file should stay open")
	}
	h.keys(ports.Ctrl('g'))
	h.typeText("400\n")
	h.typeText("X")
	if screen := strings.Join(h.screen(), "\n"); !strings.Contains(screen, "Xline 400") {
		t.Errorf("after go to line 400 and typing, screen:\n%s", screen)
	}
	if !strings.Contains(h.row(rowStatus), "Ln 400") {
		t.Errorf("status = %q", h.row(rowStatus))
	}
}

func TestSavingLargeFileReopensIt(t *testing.T) {
	text := numbered(500)
	h := newLargeHarness(t, map[string]string{"big.txt": text}, nil)
	_ = h.Dispatch("file.open", "big.txt")
	old := h.file.(*memFile)
	h.typeText("A")
	h.keys(ports.Ctrl('s'))
	if got := string(h.fs.files["big.txt"]); got != "A"+text {
		t.Fatalf("saved %d bytes, want the edit in front of the original", len(got))
	}
	if h.file == nil || h.file == ports.File(old) || !old.closed {
		t.Fatalf("expected a fresh file view and the old one closed: closed=%v", old.closed)
	}
	if h.doc.Modified() || h.doc.Err() != nil {
		t.Errorf("modified=%v err=%v", h.doc.Modified(), h.doc.Err())
	}
	// Still editable and undoable after the rebase.
	h.typeText("B")
	h.keys(ports.Ctrl('z'), ports.Ctrl('z'))
	if h.doc.Text() != text {
		t.Error("undo across a save lost text")
	}
}

func TestOpeningAnotherFileClosesTheLargeOne(t *testing.T) {
	h := newLargeHarness(t, map[string]string{"big.txt": numbered(500), "s.txt": "x"}, nil)
	_ = h.Dispatch("file.open", "big.txt")
	f := h.file.(*memFile)
	_ = h.Dispatch("file.open", "s.txt")
	if !f.closed || h.file != nil {
		t.Errorf("closed=%v file=%v", f.closed, h.file)
	}
}

func TestFileLostAsksToReload(t *testing.T) {
	text := numbered(500)
	h := newLargeHarness(t, map[string]string{"big.txt": text}, nil)
	_ = h.Dispatch("file.open", "big.txt")
	h.file.(*memFile).err = errors.New("file changed on disk")
	h.keys(ports.Ctrl('g'))
	h.typeText("300\n") // forces reads
	h.wantRow(rowPrompt, "File changed on disk (or unreadable)  [r]eload  [q]uit  Esc")
	// Any other key asks again; saving is refused meanwhile.
	h.typeText("z")
	h.wantRow(rowPrompt, "File changed on disk (or unreadable)  [r]eload  [q]uit  Esc")
	h.fs.files["big.txt"] = []byte(strings.Replace(text, "line 1\n", "reloaded\n", 1))
	h.typeText("r")
	if h.doc.Err() != nil || !strings.Contains(h.row(rowText), "reloaded") {
		t.Fatalf("after reload: err=%v row=%q", h.doc.Err(), h.row(rowText))
	}
	// q quits.
	h.file.(*memFile).err = errors.New("gone")
	h.keys(ports.Ctrl('g'))
	h.typeText("300\n")
	h.typeText("q")
	if !h.Done() {
		t.Error("q did not quit")
	}
}

func TestLargeFileIsNotValidatedOrHighlighted(t *testing.T) {
	big := "a: 1\n" + strings.Repeat("b: BAD\n", 100)
	h := newLargeHarness(t, map[string]string{"big.yaml": big}, func(c *Config) { c.SyntaxMaxSize = largeLimit })
	hl := h.cfg.Highlighter.(*fakeHighlighter)
	_ = h.Dispatch("file.open", "big.yaml")
	if len(hl.jobs) != 0 {
		t.Error("large file sent to the highlighter")
	}
	err := h.Dispatch("edit.validate")
	if err == nil || !strings.Contains(err.Error(), "too large to validate") {
		t.Errorf("validate err = %v", err)
	}
	h.typeText("#")
	h.keys(ports.Ctrl('s'))
	if !strings.HasPrefix(string(h.fs.files["big.yaml"]), "#a: 1") || h.focus != focusEditor {
		t.Fatalf("large file not saved without asking: focus %v", h.focus)
	}
	if !strings.Contains(h.row(rowPrompt), "not validated: file too large") {
		t.Errorf("prompt line = %q", h.row(rowPrompt))
	}
}

func TestCopyRefusesHugeSelection(t *testing.T) {
	h := newLargeHarness(t, map[string]string{"huge.txt": strings.Repeat("0123456789abcdef\n", maxCopySize/16+1)},
		func(c *Config) { c.LargeFileSize = 1 << 20 })
	_ = h.Dispatch("file.open", "huge.txt")
	h.keys(ports.Ctrl('a'), ports.Ctrl('c'))
	if got := h.row(rowPrompt); !strings.Contains(got, "selection too large to copy") {
		t.Errorf("prompt line = %q", got)
	}
	if h.clip != nil && h.clip.text != "" {
		t.Error("clipboard was written")
	}
}

func TestFormatSize(t *testing.T) {
	for n, want := range map[int]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KB", 1536: "1.5 KB", 32 << 20: "32.0 MB", 3 << 30: "3.0 GB"} {
		if got := formatSize(n); got != want {
			t.Errorf("formatSize(%d) = %q, want %q", n, got, want)
		}
	}
}
