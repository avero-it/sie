package app

import (
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
)

func TestLiveFindFollowsTyping(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("one\ntwo\nthree two\nfour")
	h.keys(ports.Ctrl('f'))
	h.typeText("t")
	if s, _ := h.doc.Selection().Range(); s != 4 { // wraps from the end to "two"
		t.Errorf("after t: selection starts at %d", s)
	}
	h.typeText("h")
	if s, e := h.doc.Selection().Range(); s != 8 || e != 10 {
		t.Errorf("after th: selection %d-%d", s, e)
	}
	// Backspace searches again from where the prompt opened, not from the last match.
	h.keys(key(ports.KeyBackspace))
	if s, _ := h.doc.Selection().Range(); s != 4 {
		t.Errorf("after backspace: selection starts at %d", s)
	}
}

func TestLiveFindEscRestores(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("a\nb\nc\nd")
	h.keys(key(ports.KeyUp))
	before := h.doc.Selection()
	h.keys(ports.Ctrl('f'))
	h.typeText("a")
	if h.doc.Selection() == before {
		t.Fatal("live search did not move the selection")
	}
	h.keys(key(ports.KeyEsc))
	if h.doc.Selection() != before {
		t.Errorf("Esc: selection %v, want %v", h.doc.Selection(), before)
	}
}

func TestLiveFindEnterMatchesPlainFind(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("x ab ab")
	h.keys(key(ports.KeyHome))
	h.keys(ports.Ctrl('f'))
	h.typeText("ab")
	live := h.doc.Selection()
	h.keys(key(ports.KeyEnter))
	if h.doc.Selection() != live {
		t.Errorf("Enter: selection %v, live had %v", h.doc.Selection(), live)
	}
}

func TestLiveFindNoMatchKeepsCursor(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("hello")
	before := h.doc.Selection()
	h.keys(ports.Ctrl('f'))
	h.typeText("zz")
	if h.doc.Selection() != before {
		t.Errorf("selection moved to %v", h.doc.Selection())
	}
	h.wantRowContains(rowPrompt, "not found")
}

func TestLiveFindHighlightsVisibleMatches(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("foo bar foo")
	h.keys(key(ports.KeyHome))
	h.keys(ports.Ctrl('f'))
	h.typeText("foo")
	row := h.row(1)
	x := strings.Index(row, "foo bar foo")
	if x < 0 {
		t.Fatalf("text row %q", row)
	}
	if got := h.styleAt(x, 1); got != ports.StyleSelection {
		t.Errorf("current match style %v", got)
	}
	if got := h.styleAt(x+8, 1); got != ports.StyleMatch {
		t.Errorf("other match style %v", got)
	}
	if got := h.styleAt(x+4, 1); got == ports.StyleMatch {
		t.Error("non-match highlighted")
	}
	h.keys(key(ports.KeyEsc))
	if got := h.styleAt(x+8, 1); got == ports.StyleMatch {
		t.Error("highlight survived closing the prompt")
	}
}
