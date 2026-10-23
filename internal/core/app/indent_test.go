package app

import (
	"testing"

	"github.com/avero-it/sie/internal/core/domain/editor"
	"github.com/avero-it/sie/internal/core/ports"
)

var shiftTab = ports.KeyEvent{Key: ports.KeyTab, Mod: ports.ModShift}

func TestTabAndShiftTab(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("ab")
	h.keys(key(ports.KeyTab))
	if got := h.doc.Text(); got != "ab\t" {
		t.Fatalf("tab: %q", got)
	}
	h.keys(shiftTab)
	if got := h.doc.Text(); got != "ab" {
		t.Fatalf("shift+tab should delete the tab on the left: %q", got)
	}
}

func TestTabIndentsSelectedLines(t *testing.T) {
	h := newHarness(t, nil)
	if err := h.doc.Insert("a\nb\nc"); err != nil {
		t.Fatal(err)
	}
	h.doc.SetSelection(editor.Selection{Anchor: 0, Head: 3})
	h.keys(key(ports.KeyTab))
	if got := h.doc.Text(); got != "\ta\n\tb\nc" {
		t.Fatalf("indent: %q", got)
	}
	h.keys(shiftTab)
	if got := h.doc.Text(); got != "a\nb\nc" {
		t.Fatalf("outdent: %q", got)
	}
	h.keys(ports.KeyEvent{Key: ports.KeyRune, Rune: 'z', Mod: ports.ModCtrl}) // no binding: ignored
}

func TestAltBackspaceAndDeleteRemoveWords(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("one two three")
	h.keys(ports.KeyEvent{Key: ports.KeyBackspace, Mod: ports.ModAlt})
	if got := h.doc.Text(); got != "one two " {
		t.Fatalf("back: %q", got)
	}
	h.keys(ports.KeyEvent{Key: ports.KeyLeft, Mod: ports.ModCtrl})
	h.keys(ports.KeyEvent{Key: ports.KeyDelete, Mod: ports.ModAlt})
	if got := h.doc.Text(); got != "one  " {
		t.Fatalf("forward: %q", got)
	}
	h.keys(key(ports.KeyBackspace))
	if got := h.doc.Text(); got != "one " {
		t.Fatalf("plain backspace removes one character: %q", got)
	}
}

func TestMoveLineAndSelectOccurrenceChords(t *testing.T) {
	h := newHarness(t, nil)
	if err := h.doc.Insert("a\nb\nc"); err != nil {
		t.Fatal(err)
	}
	h.keys(ports.KeyEvent{Key: ports.KeyHome, Mod: ports.ModCtrl}, key(ports.KeyDown))
	h.keys(ports.Ctrl('p'), chord('u'))
	if got := h.doc.Text(); got != "b\na\nc" {
		t.Fatalf("up: %q", got)
	}
	h.keys(ports.Ctrl('p'), chord('j'))
	if got := h.doc.Text(); got != "a\nb\nc" {
		t.Fatalf("down: %q", got)
	}
	h.wantRowContains(rowPrompt, "Move line down")

	h = newHarness(t, nil)
	h.Resize(80, 10)
	if err := h.doc.Insert("foo foo"); err != nil {
		t.Fatal(err)
	}
	h.keys(ports.KeyEvent{Key: ports.KeyHome})
	h.keys(ports.Ctrl('p'), chord('v')) // selects the word
	h.keys(ports.Ctrl('p'), chord('v')) // adds the next occurrence
	if h.doc.CursorCount() != 2 {
		t.Fatalf("cursors: %d", h.doc.CursorCount())
	}
	h.keys(ports.Ctrl('p'), chord('q')) // previous: already selected, nothing new
	h.wantRowContains(rowPrompt, "no other occurrence")
}

func TestToggleCommentNeedsLanguage(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("x")
	h.keys(ports.Ctrl('p'), chord('m'))
	h.wantRowContains(rowPrompt, "no language set")
}

func TestCommentPrefix(t *testing.T) {
	for lang, want := range map[string]string{"Go": "//", "Python": "#", "SQL": "--", "INI": ";"} {
		if got, ok := commentPrefix(lang); !ok || got != want {
			t.Errorf("%s: %q %v", lang, got, ok)
		}
	}
	for _, lang := range []string{"JSON", "HTML", "Markdown", ""} {
		if _, ok := commentPrefix(lang); ok {
			t.Errorf("%s should have no line comment", lang)
		}
	}
}
