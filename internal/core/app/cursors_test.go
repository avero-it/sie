package app

import (
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
)

// multiCursorHarness opens "ab\ncd" with cursors at the start of both lines.
func multiCursorHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, nil)
	h.typeText("ab\ncd")
	h.click(3, rowText, 0)              // gutter is 3 wide: line 1, column 0
	h.click(3, rowText+1, ports.ModAlt) // adds line 2, column 0
	if n := h.doc.CursorCount(); n != 2 {
		t.Fatalf("cursors = %d, want 2", n)
	}
	return h
}

func TestAltClickTypesAtEveryCursor(t *testing.T) {
	h := multiCursorHarness(t)
	h.typeText("xy")
	h.keys(key(ports.KeyBackspace), key(ports.KeyDelete))
	if got, want := h.doc.Text(), "xb\nxd"; got != want {
		t.Fatalf("text %q, want %q", got, want)
	}
	h.keys(ports.Ctrl('z'), ports.Ctrl('z'), ports.Ctrl('z')) // delete, backspace, typing
	if got, want := h.doc.Text(), "ab\ncd"; got != want {
		t.Errorf("undo: %q, want %q", got, want)
	}
	if n := h.doc.CursorCount(); n != 2 {
		t.Errorf("undo restored %d cursors, want 2", n)
	}
}

func TestSecondaryCursorDrawnAndCounted(t *testing.T) {
	h := multiCursorHarness(t)
	// The primary cursor is the terminal cursor; the secondary one is a cell.
	if got := h.styleAt(3, rowText); got != ports.StyleCursorSecondary {
		t.Errorf("secondary cursor style %v", got)
	}
	if vm := h.View(); vm.Cursor.X != 3 || vm.Cursor.Y != rowText+1 {
		t.Errorf("terminal cursor at %d,%d", vm.Cursor.X, vm.Cursor.Y)
	}
	h.wantRowContains(rowStatus, "(2 cursors)")
}

func TestSecondaryCursorsRemoved(t *testing.T) {
	tests := []struct {
		name string
		do   func(h *harness)
	}{
		{"esc", func(h *harness) { h.keys(key(ports.KeyEsc)) }},
		{"click", func(h *harness) { h.click(4, rowText, 0) }},
		{"command", func(h *harness) { h.keys(ports.Ctrl('a')) }},
		{"alt-click toggles", func(h *harness) { h.click(3, rowText, ports.ModAlt) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := multiCursorHarness(t)
			tt.do(h)
			if n := h.doc.CursorCount(); n != 1 {
				t.Errorf("cursors = %d, want 1", n)
			}
		})
	}
}

func TestPasteAtEveryCursor(t *testing.T) {
	h := multiCursorHarness(t)
	h.clip.text = "zz"
	h.keys(ports.Ctrl('v'))
	if got, want := h.doc.Text(), "zzab\nzzcd"; got != want {
		t.Errorf("text %q, want %q", got, want)
	}
}
